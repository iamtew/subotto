package parser

import (
	"reflect"
	"testing"
)

func TestExtractVideoIDs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "watch url",
			in:   "check https://www.youtube.com/watch?v=dQw4w9WgXcQ please",
			want: []string{"dQw4w9WgXcQ"},
		},
		{
			name: "youtu.be with timestamp",
			in:   "https://youtu.be/dQw4w9WgXcQ?t=43",
			want: []string{"dQw4w9WgXcQ"},
		},
		{
			name: "shorts",
			in:   "https://youtube.com/shorts/abcdefghijk",
			want: []string{"abcdefghijk"},
		},
		{
			name: "music subdomain",
			in:   "https://music.youtube.com/watch?v=dQw4w9WgXcQ&list=PLxyz",
			want: []string{"dQw4w9WgXcQ"},
		},
		{
			name: "embed",
			in:   "https://www.youtube.com/embed/dQw4w9WgXcQ",
			want: []string{"dQw4w9WgXcQ"},
		},
		{
			name: "multiple unique",
			in:   "https://youtu.be/dQw4w9WgXcQ and https://www.youtube.com/watch?v=aaaaaaaaaaa and again https://youtu.be/dQw4w9WgXcQ",
			want: []string{"dQw4w9WgXcQ", "aaaaaaaaaaa"},
		},
		{
			name: "no links",
			in:   "just chatting about cats",
			want: nil,
		},
		{
			name: "punctuation trailing",
			in:   "see https://youtu.be/dQw4w9WgXcQ.",
			want: []string{"dQw4w9WgXcQ"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractVideoIDs(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ExtractVideoIDs(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}
