package skills

import "testing"

func TestParseMetadata(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantName string
		wantDesc string
		wantErr  bool
	}{
		{name: "inline", content: "---\nname: review\ndescription: Review changes\n---\nBody", wantName: "review", wantDesc: "Review changes"},
		{name: "quoted", content: "---\nname: 'review'\ndescription: \"Review changes\"\n---\n", wantName: "review", wantDesc: "Review changes"},
		{name: "folded block", content: "---\nname: review\ndescription: >\n  Review changes and\n  report risks.\nlicense: Apache-2.0\n---\n", wantName: "review", wantDesc: "Review changes and report risks."},
		{name: "literal block", content: "---\nname: review\ndescription: |\n  Review changes.\n  Report risks.\n---\n", wantName: "review", wantDesc: "Review changes.\nReport risks."},
		{name: "missing frontmatter", content: "Body", wantErr: true},
		{name: "unterminated", content: "---\nname: review", wantErr: true},
		{name: "repeated", content: "---\nname: review\nname: other\n---\n", wantErr: true},
		{name: "unquoted colon", content: "---\ndescription: Use when: reviewing\n---\n", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metadata, err := parseMetadata([]byte(test.content))
			if test.wantErr {
				if err == nil {
					t.Fatalf("metadata = %#v, expected error", metadata)
				}
				return
			}
			if err != nil || metadata.name != test.wantName || metadata.description != test.wantDesc {
				t.Fatalf("metadata = %#v, err = %v", metadata, err)
			}
		})
	}
}
