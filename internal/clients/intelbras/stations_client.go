package intelbras

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Path da rota de lista de estações na AP' de terceiros (altere se for outro, ex: /stations).
const chargePointListPath = "/chargepoints"

// ChargePointListResponse é a resposta da API de lista de estações (charge points).
type ChargePointListResponse struct {
	Error            *string       `json:"error"`
	ChargePointList  []ChargePoint `json:"chargePointList"`
}

// ChargePoint representa uma estação de recarga na API de terceiros.
type ChargePoint struct {
	ChargeBoxID        string      `json:"chargeBoxId"`
	OcppProtocol       string      `json:"ocppProtocol"`
	UUID               string      `json:"uuid"`
	ChargePointVendor  string      `json:"chargePointVendor"`
	ChargePointModel   string      `json:"chargePointModel"`
	FwVersion          string      `json:"fwVersion"`
	Description        string      `json:"description"`
	Connectors         []Connector `json:"connectors"`
}

// Connector representa um conector da estação.
type Connector struct {
	ConnectorPK   int64       `json:"connectorPk"`
	ConnectorID   int         `json:"connectorId"`
	PowerMax      int         `json:"powerMax"`
	ConnectorType string      `json:"connectorType"`
	LastStatus    *LastStatus `json:"lastStatus"`
}

// LastStatus contém o último status do conector (errorCode, erroInfo, status).
type LastStatus struct {
	ErrorCode string `json:"errorCode"`
	ErroInfo  string `json:"erroInfo"`
	Status    string `json:"status"`
}

// FlattenedConnector reúne dados do conector para uso interno (inclui status do lastStatus).
type FlattenedConnector struct {
	ConnectorID   int    `json:"connectorId"`
	ConnectorPK   int64  `json:"connectorPk"`
	PowerMax      int    `json:"powerMax"`
	ConnectorType string `json:"connectorType"`
	ErrorCode     string `json:"errorCode"`
	ErroInfo      string `json:"erroInfo"`
	Status        string `json:"status"`
}

// FlattenedChargePoint é o charge point com conectores já “achatados” (status no nível do conector).
type FlattenedChargePoint struct {
	ChargeBoxID       string               `json:"chargeBoxId"`
	OcppProtocol      string               `json:"ocppProtocol"`
	UUID              string               `json:"uuid"`
	ChargePointVendor string               `json:"chargePointVendor"`
	ChargePointModel  string               `json:"chargePointModel"`
	FwVersion         string               `json:"fwVersion"`
	Description       string               `json:"description"`
	Connectors        []FlattenedConnector  `json:"connectors"`
}

// GetChargePointList faz GET na rota de estações com headers Platform, Authorization e Accept.
// Retorna a lista de charge points com os campos pedidos; conectores incluem status do lastStatus.
func (c *Client) GetChargePointList(ctx context.Context, accessToken string) (*ChargePointListResponse, error) {
	if c.chargePointLimiter != nil {
		if err := c.chargePointLimiter.Wait(ctx); err != nil {
			return nil, err
		}
	}
	url := c.baseURL + chargePointListPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create charge-points request: %w", err)
	}
	req.Header.Set("Platform", "API")
	req.Header.Set("Authorization", accessToken)
	req.Header.Set("Accept", "*/*")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute charge-points request: %w", err)
	}
	respBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read charge-points response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, newAPIError(resp.StatusCode, respBody)
	}

	var out ChargePointListResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("decode charge-points response: %w", err)
	}
	return &out, nil
}

// FlattenChargePointList converte a resposta da API em lista de FlattenedChargePoint
// (conectores com errorCode, erroInfo e status já no nível do conector).
func FlattenChargePointList(list []ChargePoint) []FlattenedChargePoint {
	result := make([]FlattenedChargePoint, 0, len(list))
	for _, cp := range list {
		fcp := FlattenedChargePoint{
			ChargeBoxID:       cp.ChargeBoxID,
			OcppProtocol:      cp.OcppProtocol,
			UUID:              cp.UUID,
			ChargePointVendor: cp.ChargePointVendor,
			ChargePointModel:  cp.ChargePointModel,
			FwVersion:         cp.FwVersion,
			Description:       cp.Description,
			Connectors:        make([]FlattenedConnector, 0, len(cp.Connectors)),
		}
		for _, conn := range cp.Connectors {
			fc := FlattenedConnector{
				ConnectorID:   conn.ConnectorID,
				ConnectorPK:   conn.ConnectorPK,
				PowerMax:      conn.PowerMax,
				ConnectorType: conn.ConnectorType,
			}
			if conn.LastStatus != nil {
				fc.ErrorCode = conn.LastStatus.ErrorCode
				fc.ErroInfo = conn.LastStatus.ErroInfo
				fc.Status = conn.LastStatus.Status
			}
			fcp.Connectors = append(fcp.Connectors, fc)
		}
		result = append(result, fcp)
	}
	return result
}
