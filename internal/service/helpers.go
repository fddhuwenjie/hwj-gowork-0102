package service

import "encoding/json"

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func mustJSONBytes(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
