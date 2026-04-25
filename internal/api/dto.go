package api

import "github.com/fakeapate/pzsm/internal/mods"

// modDTO is the wire shape of one mod. Decoupled from mods.Mod so the
// storage/service model can evolve without breaking the API contract.
type modDTO struct {
	WorkshopID string     `json:"workshop_id"`
	Name       string     `json:"name"`
	Thumbnail  string     `json:"thumbnail"`
	UpdatedAt  int64      `json:"updated_at"`
	ModIDs     []modIDDTO `json:"mod_ids"`
}

type modIDDTO struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

func toModDTO(m mods.Mod) modDTO {
	ids := make([]modIDDTO, len(m.ModIDs))
	for i, id := range m.ModIDs {
		ids[i] = modIDDTO{ID: id.ID, Enabled: id.Enabled}
	}
	return modDTO{
		WorkshopID: m.WorkshopID,
		Name:       m.Name,
		Thumbnail:  m.Thumbnail,
		UpdatedAt:  m.UpdatedAt.Unix(),
		ModIDs:     ids,
	}
}

func toModDTOs(ms []mods.Mod) []modDTO {
	out := make([]modDTO, len(ms))
	for i, m := range ms {
		out[i] = toModDTO(m)
	}
	return out
}
