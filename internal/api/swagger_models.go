package api

// HealthResponse representa a resposta do health check.
type HealthResponse struct {
	Status string `json:"status" example:"ok"`
}

// ErrorResponse representa o formato padrão de erro da API.
type ErrorResponse struct {
	Error string `json:"error" example:"unauthorized"`
}

// ConfigResponse é o JWT curto para conectar ao WebSocket (POST /v1/config ou GET /v1/ws/token).
type ConfigResponse struct {
	Token     string `json:"token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	ExpiresIn int    `json:"expiresIn" example:"300"`
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
