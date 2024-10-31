package node

import "path/filepath"

func asbPath(rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}

	rel, _ = filepath.Abs(rel)
	return rel
}
