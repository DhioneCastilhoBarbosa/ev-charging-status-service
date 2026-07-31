package api

// HealthResponse representa a resposta do health check.
type HealthResponse struct {
	Status string `json:"status" example:"ok"`
}

// ErrorResponse representa o formato padrão de erro da API.
type ErrorResponse struct {
	Error string `json:"error" example:"unauthorized"`
}

// ConfigResponse contém o JWT de sessão (resposta de POST /v1/config).
// A sessão não usa expiresIn: permanece válida até delete do usuário ou idle sem tráfego de app no WebSocket.
type ConfigResponse struct {
	Token string `json:"token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
}

// ConfigStatusResponse representa o status da configuração.
type ConfigStatusResponse struct {
	Configured     bool   `json:"configured" example:"true"`
	TokenPresent   bool   `json:"tokenPresent" example:"true"`
	TokenExpiresAt string `json:"tokenExpiresAt,omitempty" example:"2026-03-25T10:30:00Z"`
	APIUsername    string `json:"apiUsername,omitempty" example:"usuario@empresa.com"`
}

// StationsResponse representa o retorno de estações.
type StationsResponse struct {
	// Estrutura varia conforme payload da API Move/Intelbras.
	Stations []map[string]interface{} `json:"stations"`
}

// StationsPushResponse é o JSON retornado por POST /v1/stations e pelos frames WebSocket de estações.
type StationsPushResponse struct {
	UserID    string               `json:"userId" example:"550e8400-e29b-41d4-a716-446655440000"`
	Timestamp string               `json:"timestamp" example:"2026-05-04T12:00:00Z"`
	Stations  []StationsPushStation `json:"stations"`
}

// StationsPushStation representa uma estação no payload de estações.
type StationsPushStation struct {
	ChargeBoxID       string                 `json:"chargeBoxId"`
	Description       string                 `json:"description"`
	OcppProtocol      string                 `json:"ocppProtocol"`
	ChargePointModel  string                 `json:"chargePointModel"`
	ChargePointVendor string                 `json:"chargePointVendor"`
	UUID              string                 `json:"uuid"`
	FwVersion         string                 `json:"fwVersion"`
	Connectors        []StationsPushConnector `json:"connectors"`
}

// StationsPushConnector representa um conector no payload de estações.
type StationsPushConnector struct {
	Status        string `json:"status"`
	ErroInfo      string `json:"erroInfo"`
	PowerMax      int    `json:"powerMax"`
	ErrorCode     string `json:"errorCode"`
	ConnectorID   int    `json:"connectorId"`
	ConnectorPK   int64  `json:"connectorPk"`
	ConnectorType string `json:"connectorType"`
}
