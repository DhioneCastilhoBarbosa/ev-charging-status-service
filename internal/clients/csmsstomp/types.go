package csmsstomp

// StatusEvent é o JSON publicado pelo CSMS em /topic/status/chargeBox/{uuid}.
type StatusEvent struct {
	ChargeBoxID              string  `json:"chargeBoxId"`
	ChargeBoxUUID            string  `json:"chargeBoxUuid"`
	ConnectorID              int     `json:"connectorId"`
	TimestampDT              string  `json:"timestampDT"`
	Status                   string  `json:"status"`
	ErrorCode                string  `json:"errorCode"`
	ChargeBoxGroupPk         int     `json:"chargeBoxGroupPk"`
	ChargeBoxGroupExternalID string  `json:"chargeBoxGroupExternalId"`
	ErrorInfo                *string `json:"errorInfo"`
	VendorID                 *string `json:"vendorId"`
	VendorErrorCode          *string `json:"vendorErrorCode"`
}
