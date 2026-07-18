package activate

import (
	"testing"
	"zsh-pro/core/model"
)

func TestDiffRejectsUnsupportedSchemas(t *testing.T) {
	for _, bad := range []struct {
		name           string
		active, target *model.Manifest
	}{
		{"active", &model.Manifest{Schema: "v2"}, &model.Manifest{Schema: model.SchemaV1}},
		{"target", &model.Manifest{Schema: model.SchemaV1}, &model.Manifest{Schema: "v2"}},
	} {
		t.Run(bad.name, func(t *testing.T) {
			p, err := Diff(bad.active, bad.target)
			if err == nil || len(p.Deactivate) != 0 || len(p.Activate) != 0 {
				t.Fatalf("plan=%#v err=%v", p, err)
			}
		})
	}
}
