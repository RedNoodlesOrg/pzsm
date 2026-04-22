package steam

import (
	"reflect"
	"testing"
)

func TestExtractModIDs(t *testing.T) {
	cases := []struct {
		name string
		desc string
		want []ModID
	}{
		{
			name: "single id auto-enabled",
			desc: "Some description.\nMod ID: MyMod\nWorkshop ID: 12345",
			want: []ModID{{ID: "MyMod", Enabled: true}},
		},
		{
			name: "multiple ids all disabled and sorted by length then lex",
			desc: "Mod ID: Foo\nMod ID: Bar\nMod ID: Bazzz\nMod ID: Quxx",
			want: []ModID{
				{ID: "Bar", Enabled: false},
				{ID: "Foo", Enabled: false},
				{ID: "Quxx", Enabled: false},
				{ID: "Bazzz", Enabled: false},
			},
		},
		{
			name: "duplicates deduped",
			desc: "Mod ID: Same\nMod ID: Same\nMod ID: Same",
			want: []ModID{{ID: "Same", Enabled: true}},
		},
		{
			name: "bbcode close-bold tolerated",
			desc: "Mod ID: [/b] Clean",
			want: []ModID{{ID: "Clean", Enabled: true}},
		},
		{
			name: "case insensitive on the label, preserves the id case",
			desc: "mod id: Lower\nMOD ID: Upper\nMod Id: Mixed",
			want: []ModID{
				{ID: "Lower", Enabled: false},
				{ID: "Mixed", Enabled: false},
				{ID: "Upper", Enabled: false},
			},
		},
		{
			name: "no ids returns empty slice",
			desc: "description without a mod id reference",
			want: nil,
		},
		{
			name: "blank match skipped",
			desc: "Mod ID: \nMod ID: Real",
			want: []ModID{{ID: "Real", Enabled: true}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractModIDs(tc.desc)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("\n got:  %+v\n want: %+v", got, tc.want)
			}
		})
	}
}
