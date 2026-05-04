// Package main inicia a API HTTP do EV Charging Status Service.
//
//	@title			EV Charging Status API
//	@version		1.0
//	@description	API para configuração e consulta de estações de recarga (Move/Intelbras), `POST /v1/stations` com JWT, webhook opcional no worker. Rotas em tempo real fora desta especificação permanecem disponíveis no serviço.
//	@host			defense.intelbras-cve-pro.com.br
//	@schemes		https
//	@BasePath		/
//	@securityDefinitions.apikey	ApiKeyAuth
//	@in							header
//	@name						X-API-Key
package main

import (
	"log"
	"os"

	_ "ev-charging-status-service/docs"

	"ev-charging-status-service/internal/api"
	"ev-charging-status-service/internal/config"
	"ev-charging-status-service/internal/database"
)

func main() {
	cfg := config.Load()
	if cfg.APIKey == "" && os.Getenv("ALLOW_EMPTY_API_KEY") != "true" {
		log.Fatal("API_KEY is required")
	}

	db := database.ConnectPostgres(cfg.PostgresURL)

	router := api.SetupRoutes(db, cfg)

	log.Println("API running on :8085")
	if err := router.Run(":8085"); err != nil {
		log.Fatal(err)
	}
}
