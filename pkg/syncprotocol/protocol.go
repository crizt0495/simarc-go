package syncprotocol

// PushPayload is the batch payload sent from client to server.
type PushPayload struct {
	BatchID       string                 `json:"batch_id"`
	DeviceNodeID  string                 `json:"device_node_id"`
	Items         []SyncItem             `json:"items"`
	OperationIDFixed string            `json:"operation_id_fixed"`
}

// SyncItem represents one queued change from sync_queue.
type SyncItem struct {
	OperationID string          `json:"operation_id"`
	TableName   string          `json:"table_name"`
	RecordID    string          `json:"record_id"`
	Operation   string          `json:"operation"`
	Payload     map[string]interface{} `json:"payload"`
}

// PullResponse is what server returns for a pull request.
type PullResponse struct {
	Changes []ChangeEntry `json:"changes"`
	Cursor  string        `json:"cursor"`
}

// ChangeEntry is a remote change applied locally.
type ChangeEntry struct {
	TableName string                 `json:"table_name"`
	RecordID  string                 `json:"record_id"`
	Operation string                 `json:"operation"`
	Payload   map[string]interface{} `json:"payload"`
	Sequence  int64                  `json:"sequence"`
}

// ACKResponse is server acknowledgment after successful push commit.
type ACKResponse struct {
	BatchID     string `json:"batch_id"`
	Acknowledged bool  `json:"acknowledged"`
	ServerSequence int64 `json:"server_sequence"`
}
