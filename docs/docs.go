// Package docs contém a documentação Swagger da API.
// Para regenerar: go install github.com/swaggo/swag/cmd/swag@latest && swag init -g cmd/api/main.go -o docs
package docs

import "github.com/swaggo/swag"

const docTemplate = `{
    "schemes": {{ marshal .Schemes }},
    "swagger": "2.0",
    "info": {
        "description": "{{escape .Description}}",
        "title": "{{.Title}}",
        "version": "{{.Version}}"
    },
    "host": "{{.Host}}",
    "basePath": "{{.BasePath}}",
    "paths": {
        "/health": {
            "get": {
                "description": "Health check do serviço",
                "produces": ["application/json"],
                "summary": "Health check",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "object",
                            "properties": {
                                "status": { "type": "string", "example": "ok" }
                            }
                        }
                    }
                }
            }
        },
        "/v1/config": {
            "post": {
                "description": "Configura credenciais Move/Intelbras e URL do webhook. Faz login na API de terceiros e persiste os dados.",
                "consumes": ["application/json"],
                "produces": ["application/json"],
                "summary": "Configura credenciais e webhook",
                "parameters": [
                    {
                        "in": "body",
                        "name": "body",
                        "required": true,
                        "schema": {
                            "type": "object",
                            "required": ["password", "webhookUrl"],
                            "properties": {
                                "email": { "type": "string" },
                                "username": { "type": "string" },
                                "password": { "type": "string" },
                                "recaptchaResponse": { "type": "string" },
                                "apiKey": { "type": "string" },
                                "webhookUrl": { "type": "string", "format": "uri" }
                            }
                        }
                    }
                ],
                "responses": {
                    "204": { "description": "No content" },
                    "400": { "description": "invalid request" },
                    "401": { "description": "unauthorized" },
                    "502": { "description": "configuration failed" }
                },
                "security": [{ "ApiKeyAuth": [] }]
            }
            ,
            "delete": {
                "description": "Remove o usuário indicado (email/username) e todos os dados relacionados (credenciais, webhooks e eventos) via cascade.",
                "summary": "Remove configuração e dados do usuário",
                "consumes": ["application/json"],
                "parameters": [
                    {
                        "in": "body",
                        "name": "body",
                        "required": true,
                        "schema": {
                            "type": "object",
                            "properties": {
                                "email": { "type": "string" },
                                "username": { "type": "string" }
                            }
                        }
                    }
                ],
                "responses": {
                    "204": { "description": "No content" },
                    "401": { "description": "unauthorized" }
                },
                "security": [{ "ApiKeyAuth": [] }]
            }
        },
        "/v1/config/status": {
            "get": {
                "description": "Retorna se há configuração e se o token está presente (sem expor o token).",
                "produces": ["application/json"],
                "summary": "Status da configuração",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "object",
                            "properties": {
                                "configured": { "type": "boolean" },
                                "tokenPresent": { "type": "boolean" },
                                "tokenExpiresAt": { "type": "string" },
                                "apiUsername": { "type": "string" }
                            }
                        }
                    },
                    "401": { "description": "unauthorized" },
                    "500": { "description": "configuration unavailable" }
                },
                "security": [{ "ApiKeyAuth": [] }]
            }
        },
        "/v1/stations": {
            "get": {
                "description": "Lista estações de recarga e conectores da API Move/Intelbras (usa token salvo ou renova).",
                "produces": ["application/json"],
                "summary": "Lista estações",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "object",
                            "properties": {
                                "stations": {
                                    "type": "array",
                                    "items": { "type": "object" }
                                }
                            }
                        }
                    },
                    "401": { "description": "unauthorized" },
                    "502": { "description": "stations unavailable" }
                },
                "security": [{ "ApiKeyAuth": [] }]
            }
        }
    },
    "securityDefinitions": {
        "ApiKeyAuth": {
            "type": "apiKey",
            "name": "X-API-Key",
            "in": "header"
        }
    }
}`

// SwaggerInfo holds exported Swagger Info
var SwaggerInfo = &swag.Spec{
	Version:          "1.0",
	Host:             "localhost:8080",
	BasePath:         "/",
	Schemes:          []string{},
	Title:            "EV Charging Status API",
	Description:      "API para configuração e consulta de estações de recarga (Move/Intelbras) e envio por webhook.",
	InfoInstanceName: "swagger",
	SwaggerTemplate:  docTemplate,
}

func init() {
	swag.Register(SwaggerInfo.InstanceName(), SwaggerInfo)
}
