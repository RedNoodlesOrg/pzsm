package steam

import (
	"reflect"
	"testing"
)

func TestExtractModIDs(t *testing.T) {
	cases := []struct {
		name string
		desc string
		want []string
	}{
		{
			name: "single id",
			desc: "Some description.\nMod ID: MyMod\nWorkshop ID: 12345",
			want: []string{"MyMod"},
		},
		{
			name: "multiple ids sorted by length then lex",
			desc: "Mod ID: Foo\nMod ID: Bar\nMod ID: Bazzz\nMod ID: Quxx",
			want: []string{"Bar", "Foo", "Quxx", "Bazzz"},
		},
		{
			name: "duplicates deduped",
			desc: "Mod ID: Same\nMod ID: Same\nMod ID: Same",
			want: []string{"Same"},
		},
		{
			name: "bbcode close-bold tolerated",
			desc: "Mod ID: [/b] Clean",
			want: []string{"Clean"},
		},
		{
			name: "case insensitive on the label, preserves the id case",
			desc: "mod id: Lower\nMOD ID: Upper\nMod Id: Mixed",
			want: []string{"Lower", "Mixed", "Upper"},
		},
		{
			name: "no ids returns empty slice",
			desc: "description without a mod id reference",
			want: nil,
		},
		{
			name: "blank match skipped",
			desc: "Mod ID: \nMod ID: Real",
			want: []string{"Real"},
		},
		{
			name: "OLD MOD ID label is rejected",
			desc: "Mod ID: New\nOLD MOD ID: Stale",
			want: []string{"New"},
		},
		{
			name: "version prefix accepted",
			desc: "v1.1 Mod ID: snowiswater",
			want: []string{"snowiswater"},
		},
		{
			name: "build prefix accepted",
			desc: "B41 Mod ID: foo\nB42 Mod ID: bar",
			want: []string{"bar", "foo"},
		},
		{
			name: "parenthetical between ID and colon",
			desc: "Mod ID (b41): ForB41\nMod ID (b42): ForB42",
			want: []string{"ForB41", "ForB42"},
		},
		{
			name: "trailing BBCode stripped from captured id",
			desc: "Mod ID: SlowConsumption[/b]",
			want: []string{"SlowConsumption"},
		},
		{
			name: "inline BBCode annotation stripped",
			desc: "Mod ID: SpecialEmergencyVehicles  [b](CHANGED)[/b]",
			want: []string{"SpecialEmergencyVehicles"},
		},
		{
			name: "leading bbcode wrapper around the label",
			desc: "[b]Mod ID:[/b] Wrapped",
			want: []string{"Wrapped"},
		},
		{
			name: "plural Mod IDs heading is skipped",
			desc: "[b]Mod IDs:[/b] First | Second\nMod ID: First\nMod ID: Second",
			want: []string{"First", "Second"},
		},
		{
			name: "mid-prose mod id mention is rejected",
			desc: "Make sure to set the same Mod ID: across servers.\nMod ID: Real",
			want: []string{"Real"},
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
