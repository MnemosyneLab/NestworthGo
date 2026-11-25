package lucide

import "testing"

func TestGeneratedIconsAreWellFormedAndInsideTheGrid(t *testing.T) {
	arity := map[int]int{cmdMove: 2, cmdLine: 2, cmdCubic: 6, cmdClose: 0}
	for n, icon := range All() {
		if icon == nil || len(icon.paths) == 0 {
			t.Fatalf("icon %d has no paths", n)
		}
		for _, path := range icon.paths {
			if cmd := int(path[0]); cmd != cmdMove {
				t.Fatalf("icon %d: a path must start with a move, got command %d", n, cmd)
			}
			for i := 0; i < len(path); {
				size, ok := arity[int(path[i])]
				if !ok || i+1+size > len(path) {
					t.Fatalf("icon %d: malformed command %v at %d", n, path[i], i)
				}
				for _, v := range path[i+1 : i+1+size] {
					// Round caps may reach just past the 24 unit grid.
					if v < -1 || v > gridSize+1 {
						t.Fatalf("icon %d: coordinate %v is far outside the %d unit grid", n, v, gridSize)
					}
				}
				i += 1 + size
			}
		}
	}
}
