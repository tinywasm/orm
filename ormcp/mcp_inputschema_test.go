package ormcp

import (
	"strings"
	"testing"

	"github.com/tinywasm/context"
	"github.com/tinywasm/json"
	"github.com/tinywasm/mcp"
	"github.com/tinywasm/model"
)

// encodeMCPMessage serializes an mcp.JSONRPCMessage to its wire JSON using
// tinywasm/json (never stdlib), exactly as the transport emits it.
func encodeMCPMessage(resp mcp.JSONRPCMessage) string {
	var b []byte
	if f, ok := resp.(model.Encodable); ok {
		_ = json.Encode(f, &b)
	}
	return string(b)
}

// TestMCP_ToolsList_InputSchemaIsValidJSONSchema garantiza que las tools db_*
// expongan un inputSchema JSON Schema "object" VÁLIDO en tools/list.
//
// Contrato MCP: inputSchema DEBE ser un objeto JSON Schema. Claude Code valida
// tools/list con Zod y descarta el array COMPLETO si uno es inválido — dejando el
// servidor "Connected" pero sin tools. Este test FALLA mientras el schema se genere
// mal (db_query/db_exec serializando el struct → {"SQL":""}; db_schema/db_export_schema
// con "" → null) y PASA cuando mcp genera el inputSchema desde Tool.Args (Schema()).
// NO debe existir lógica de JSON Schema en ormcp.
func TestMCP_ToolsList_InputSchemaIsValidJSONSchema(t *testing.T) {
	srv, err := mcp.NewServer(mcp.Config{Name: "test", Version: "1.0.0", Authorize: mcp.AllowAll}, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	tools := NewDaemonProvider().Tools()
	for _, tool := range tools {
		if e := srv.AddTool(tool); e != nil {
			t.Fatalf("AddTool %s: %v", tool.Name, e)
		}
	}

	var ctx context.Context
	ctx.Set(mcp.CtxKeyUserID, "u1")
	resp := srv.HandleMessage(&ctx, []byte(`{"jsonrpc":"2.0","id":"1","method":"tools/list","params":{}}`))
	if resp == nil {
		t.Fatal("tools/list devolvió nil")
	}
	body := encodeMCPMessage(resp)

	if strings.Contains(body, `"inputSchema":null`) {
		t.Errorf("hay tools con inputSchema:null (inválido)\nbody: %s", body)
	}
	got := strings.Count(body, `"inputSchema":{"type":"object"`)
	if got != len(tools) {
		t.Errorf("inputSchema \"object\" válidos = %d, se esperaban %d (uno por tool)\nbody: %s",
			got, len(tools), body)
	}
	// db_query / db_exec deben exponer SQL como string, no el struct serializado.
	if strings.Contains(body, `"inputSchema":{"SQL"`) {
		t.Errorf("inputSchema es el struct serializado, no JSON Schema\nbody: %s", body)
	}
	if !strings.Contains(body, `"SQL":{"type":"string"}`) {
		t.Errorf("se esperaba properties.SQL como string\nbody: %s", body)
	}
}
