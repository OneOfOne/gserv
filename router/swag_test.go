package router

import (
	"encoding/json"
	"os"
	"testing"

	"go.oneofone.dev/genh"
)

func TestSwagger(t *testing.T) {
	var r Router
	b, _ := os.ReadFile("/tmp/swagger-apr-26-2023-12h-54m.json")
	json.Unmarshal(b, &r.swagger)
	swg := &r.swagger
	j, _ := json.Marshal(swg)
	c := genh.Clone(swg, true)
	j2, _ := json.Marshal(c)
	t.Log(string(j2) == string(j))
}
