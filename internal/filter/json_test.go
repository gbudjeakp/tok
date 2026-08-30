package filter

import (
	"strings"
	"testing"
)

func TestSmashJSON_Array(t *testing.T) {
	in := `[{"id":1,"name":"alice"},{"id":2,"name":"bob"}]`
	out, ok := SmashJSON(in)
	if !ok {
		t.Fatal("expected ok=true for JSON array")
	}
	if !strings.Contains(out, "[2 items]") {
		t.Errorf("expected item count header, got: %s", out)
	}
	if !strings.Contains(out, "id") || !strings.Contains(out, "name") {
		t.Errorf("expected column headers, got: %s", out)
	}
}

func TestSmashJSON_Object(t *testing.T) {
	in := `{"status":"ok","code":200,"data":"hello"}`
	out, ok := SmashJSON(in)
	if !ok {
		t.Fatal("expected ok=true for JSON object")
	}
	if !strings.Contains(out, "status") {
		t.Errorf("expected keys in output, got: %s", out)
	}
}

func TestSmashJSON_NotJSON(t *testing.T) {
	_, ok := SmashJSON("not json at all")
	if ok {
		t.Fatal("expected ok=false for non-JSON input")
	}
}

func TestSmashJSON_Empty(t *testing.T) {
	_, ok := SmashJSON("")
	if ok {
		t.Fatal("expected ok=false for empty input")
	}
}

func TestSmashJSON_EmptyArray(t *testing.T) {
	_, ok := SmashJSON("[]")
	if ok {
		t.Fatal("expected ok=false for empty array")
	}
}

func TestSmashJSON_EmptyObject(t *testing.T) {
	_, ok := SmashJSON("{}")
	if ok {
		t.Fatal("expected ok=false for empty object")
	}
}

func TestSmashJSON_LargeArray_Truncated(t *testing.T) {
	// Build array with 20 items — should show "… N more" footer.
	var items []string
	for i := 0; i < 20; i++ {
		items = append(items, `{"n":1}`)
	}
	in := "[" + strings.Join(items, ",") + "]"
	out, ok := SmashJSON(in)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(out, "more") {
		t.Errorf("expected truncation footer, got: %s", out)
	}
}
