#!/usr/bin/env python3
"""Investiga estação OSJ0008305251 e tenta consultar API Move."""
import base64
import hashlib
import json
import os
import sys
import urllib.error
import urllib.request
from datetime import datetime, timezone

import psycopg2

STATION_ID = "OSJ0008305251"
TRANSACTION_ID = "519871"
BASE_URL = "https://cs-test.use-move.com/api/v1"
API_KEY = os.environ.get("API_KEY", "")
ENCRYPTION_KEY = os.environ.get("ENCRYPTION_KEY", "")
POSTGRES_URL = os.environ.get(
    "POSTGRES_URL",
    "postgresql://cve-intelbras:Intelbrascve2026@54.159.164.244:5438/ev-status-db-defense?sslmode=disable",
)


def decrypt_aes_gcm(ciphertext_b64: str, key: bytes) -> str:
    """Compatível com internal/crypto do ev-charging-status-service."""
    try:
        from cryptography.hazmat.primitives.ciphers.aead import AESGCM
    except ImportError:
        return ciphertext_b64
    raw = base64.b64decode(ciphertext_b64)
    if len(raw) < 12 + 16:
        return ciphertext_b64
    nonce, ct = raw[:12], raw[12:]
    key32 = hashlib.sha256(key).digest()
    try:
        return AESGCM(key32).decrypt(nonce, ct, None).decode("utf-8")
    except Exception:
        return ciphertext_b64


def http_json(method: str, url: str, headers: dict, body: dict | None = None):
    data = None if body is None else json.dumps(body).encode("utf-8")
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return json.loads(resp.read().decode("utf-8")), resp.status
    except urllib.error.HTTPError as e:
        payload = e.read().decode("utf-8", errors="replace")
        return {"error": payload, "status": e.code}, e.code


def query_db():
    print("=== BANCO LOCAL (ev-status-db-defense) ===")
    conn = psycopg2.connect(POSTGRES_URL)
    cur = conn.cursor()
    cur.execute(
        """
        SELECT external_station_id, name, id, created_at
        FROM stations
        WHERE external_station_id ILIKE %s OR external_station_id ILIKE %s
        """,
        (f"%{STATION_ID}%", "%8305251%"),
    )
    stations = cur.fetchall()
    print(f"Estações encontradas: {len(stations)}")
    for ext_id, name, sid, created in stations:
        print(f"  - {ext_id} | name={name} | id={sid} | created={created}")
        cur.execute(
            """
            SELECT connector_id, status, received_at
            FROM connector_status
            WHERE station_id = %s
            ORDER BY received_at DESC
            LIMIT 15
            """,
            (sid,),
        )
        for cid, status, received in cur.fetchall():
            print(f"      connector={cid} status={status} at={received}")

    cur.execute(
        """
        SELECT user_id, api_username, api_password, api_key, access_token, token_expires_at
        FROM third_party_credentials
        LIMIT 5
        """
    )
    creds = []
    for row in cur.fetchall():
        user_id, username, password, api_key, token, expires = row
        if ENCRYPTION_KEY:
            password = decrypt_aes_gcm(password, ENCRYPTION_KEY.encode())
            if api_key:
                api_key = decrypt_aes_gcm(api_key, ENCRYPTION_KEY.encode())
        creds.append(
            {
                "user_id": str(user_id),
                "username": username,
                "password": password,
                "api_key": api_key or API_KEY,
                "token": token,
                "expires": expires,
            }
        )
        print(f"  cred user={username} token_expires={expires}")
    conn.close()
    return creds


def find_station_in_move(access_token: str, api_key: str):
    print("\n=== API MOVE: GET /chargepoints ===")
    headers = {
        "Authorization": f"Bearer {access_token}",
        "API-Key": api_key,
        "Accept": "application/json",
        "Platform": "WEB",
    }
    data, status = http_json("GET", f"{BASE_URL}/chargepoints", headers)
    if status != 200:
        print(f"Erro chargepoints HTTP {status}: {json.dumps(data, ensure_ascii=False)[:500]}")
        return None
    cps = data.get("chargePointList") or []
    print(f"Total charge points: {len(cps)}")
    match = None
    for cp in cps:
        cbid = cp.get("chargeBoxId", "")
        if STATION_ID in cbid or "8305251" in cbid:
            match = cp
            break
    if not match:
        print(f"Estação {STATION_ID} NÃO encontrada na lista.")
        return None
    print(json.dumps(match, indent=2, ensure_ascii=False))
    return match


def probe_transaction_endpoints(access_token: str, api_key: str, uuid: str):
    print("\n=== PROBANDO ENDPOINTS DE TRANSAÇÃO (descoberta) ===")
    headers = {
        "Authorization": f"Bearer {access_token}",
        "API-Key": api_key,
        "Accept": "application/json",
        "Platform": "WEB",
    }
    candidates = [
        f"{BASE_URL}/transactions/{TRANSACTION_ID}",
        f"{BASE_URL}/transaction/{TRANSACTION_ID}",
        f"{BASE_URL}/chargepoints/{uuid}/transactions/{TRANSACTION_ID}",
        f"{BASE_URL}/chargepoints/{uuid}/transactions",
        f"{BASE_URL}/chargingprofiles?chargeBoxId={STATION_ID}",
        f"{BASE_URL}/chargingprofiles?chargeBoxUuid={uuid}",
        f"{BASE_URL}/smartcharging?chargeBoxId={STATION_ID}",
    ]
    for url in candidates:
        data, status = http_json("GET", url, headers)
        short = json.dumps(data, ensure_ascii=False)
        if len(short) > 300:
            short = short[:300] + "..."
        print(f"  [{status}] {url.split('/api/v1/')[-1]} -> {short}")


def login_move(email: str, password: str, api_key: str):
    headers = {
        "Content-Type": "application/json",
        "API-Key": api_key,
        "Accept": "application/json",
        "Platform": "WEB",
    }
    body = {
        "email": email,
        "password": password,
        "recaptchaResponse": "",
    }
    data, status = http_json("POST", f"{BASE_URL}/login", headers, body)
    if status != 200:
        print(f"Login falhou HTTP {status}: {data}")
        return None
    token = data.get("accessToken") or data.get("token")
    print(f"Login OK, token len={len(token or '')}")
    return token


def main():
    if not API_KEY:
        print("Defina API_KEY no ambiente ou use .env")
    creds = query_db()
    if not creds:
        print("Sem credenciais no banco. Não é possível consultar Move API.")
        return 1
    c = creds[0]
    api_key = c["api_key"] or API_KEY
    token = c["token"]
    if c["expires"] and c["expires"].replace(tzinfo=timezone.utc) < datetime.now(timezone.utc):
        print("Token expirado, fazendo login...")
        token = None
    if not token:
        token = login_move(c["username"], c["password"], api_key)
    if not token:
        return 1
    cp = find_station_in_move(token, api_key)
    if cp:
        probe_transaction_endpoints(token, api_key, cp.get("uuid", ""))
    return 0


if __name__ == "__main__":
    # carrega .env simples
    env_path = os.path.join(os.path.dirname(__file__), "..", ".env")
    if os.path.exists(env_path):
        with open(env_path, encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if line and not line.startswith("#") and "=" in line:
                    k, v = line.split("=", 1)
                    os.environ.setdefault(k.strip(), v.strip().strip('"'))
    API_KEY = os.environ.get("API_KEY", API_KEY)
    ENCRYPTION_KEY = os.environ.get("ENCRYPTION_KEY", ENCRYPTION_KEY)
    POSTGRES_URL = os.environ.get("POSTGRES_URL", POSTGRES_URL)
    sys.exit(main())
