package server

type SetRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type SetResponse struct {
	Message string `json:"message"`
	Key     string `json:"key"`
	Value   string `json:"value"`
	Version int64  `json:"version"`
}

type GetResponse struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Version int64  `json:"version"`
	Node    string `json:"node"`
}

type ReplicateRequest struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Version int64  `json:"version"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}