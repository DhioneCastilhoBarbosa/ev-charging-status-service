package service

import (
	"bytes"
	"encoding/json"

	"ev-charging-status-service/internal/clients/intelbras"
)

func quoteString(s string) []byte {
	b, _ := json.Marshal(s)
	return b
}

// StationsPushPayload é o JSON de POST /v1/stations e dos frames WebSocket (userId, stations, timestamp).
type StationsPushPayload struct {
	UserID    string                `json:"userId"`
	Timestamp string                `json:"timestamp"`
	Stations  []StationsPushStation `json:"stations"`
}

// StationsPushStation representa uma estação no payload (connectors por último).
type StationsPushStation struct {
	ChargeBoxID       string                  `json:"chargeBoxId"`
	Description       string                  `json:"description"`
	OcppProtocol      string                  `json:"ocppProtocol"`
	ChargePointModel  string                  `json:"chargePointModel"`
	ChargePointVendor string                  `json:"chargePointVendor"`
	UUID              string                  `json:"uuid"`
	FwVersion         string                  `json:"fwVersion"`
	Connectors        []StationsPushConnector `json:"connectors"`
}

// StationsPushConnector representa um conector no payload.
type StationsPushConnector struct {
	Status        string `json:"status"`
	ErroInfo      string `json:"erroInfo"`
	PowerMax      int    `json:"powerMax"`
	ErrorCode     string `json:"errorCode"`
	ConnectorID   int    `json:"connectorId"`
	ConnectorPK   int64  `json:"connectorPk"`
	ConnectorType string `json:"connectorType"`
}

// buildStationsPushPayloadJSON monta o JSON com ordem fixa: userId, stations, timestamp.
func buildStationsPushPayloadJSON(p StationsPushPayload) ([]byte, error) {
	buf := &bytes.Buffer{}
	buf.WriteString(`{"userId":`)
	buf.Write(quoteString(p.UserID))
	buf.WriteString(`,"stations":[`)
	for i, st := range p.Stations {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(buildStationsPushStationJSON(st))
	}
	buf.WriteString(`],"timestamp":`)
	buf.Write(quoteString(p.Timestamp))
	buf.WriteString(`}`)
	return buf.Bytes(), nil
}

func buildStationsPushStationJSON(s StationsPushStation) []byte {
	buf := &bytes.Buffer{}
	buf.WriteString(`{"chargeBoxId":`)
	buf.Write(quoteString(s.ChargeBoxID))
	buf.WriteString(`,"description":`)
	buf.Write(quoteString(s.Description))
	buf.WriteString(`,"ocppProtocol":`)
	buf.Write(quoteString(s.OcppProtocol))
	buf.WriteString(`,"chargePointModel":`)
	buf.Write(quoteString(s.ChargePointModel))
	buf.WriteString(`,"chargePointVendor":`)
	buf.Write(quoteString(s.ChargePointVendor))
	buf.WriteString(`,"uuid":`)
	buf.Write(quoteString(s.UUID))
	buf.WriteString(`,"fwVersion":`)
	buf.Write(quoteString(s.FwVersion))
	buf.WriteString(`,"connectors":`)
	connEnc, _ := json.Marshal(s.Connectors)
	buf.Write(connEnc)
	buf.WriteString(`}`)
	return buf.Bytes()
}

func flattenToStationsPushStations(list []intelbras.FlattenedChargePoint) []StationsPushStation {
	out := make([]StationsPushStation, 0, len(list))
	for _, cp := range list {
		conns := make([]StationsPushConnector, 0, len(cp.Connectors))
		for _, c := range cp.Connectors {
			conns = append(conns, StationsPushConnector{
				Status:        c.Status,
				ErroInfo:      c.ErroInfo,
				PowerMax:      c.PowerMax,
				ErrorCode:     c.ErrorCode,
				ConnectorID:   c.ConnectorID,
				ConnectorPK:   c.ConnectorPK,
				ConnectorType: c.ConnectorType,
			})
		}
		out = append(out, StationsPushStation{
			ChargeBoxID:       cp.ChargeBoxID,
			Description:       cp.Description,
			OcppProtocol:      cp.OcppProtocol,
			ChargePointModel:  cp.ChargePointModel,
			ChargePointVendor: cp.ChargePointVendor,
			UUID:              cp.UUID,
			FwVersion:         cp.FwVersion,
			Connectors:        conns,
		})
	}
	return out
}
