package sqlite

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

func sqliteDSN(path string, query url.Values) (string, error) {
	if path == ":memory:" {
		return path + "?" + query.Encode(), nil
	}
	fileURL, err := sqliteFileURL(path)
	if err != nil {
		return "", err
	}
	return fileURL + "?" + query.Encode(), nil
}

func sqliteFileURL(path string) (string, error) {
	// Keep relative names in the URI path: file://name.db parses name.db as an
	// authority and modernc.org/sqlite rejects it instead of opening CWD/name.db.
	uri := url.URL{Scheme: "file", OmitHost: !filepath.IsAbs(path)}
	uriPath := filepath.ToSlash(path)
	if runtime.GOOS == "windows" && filepath.IsAbs(path) {
		volume := filepath.VolumeName(path)
		switch {
		case len(volume) == 2 && volume[1] == ':':
			// A drive-letter path in a file URI starts with /C:/ so C: is
			// part of the path rather than a URI authority.
			uriPath = "/" + uriPath
		case strings.HasPrefix(volume, `\\`):
			// Represent a UNC path as file://server/share/path.
			volumePath := filepath.ToSlash(volume)
			parts := strings.SplitN(strings.TrimPrefix(volumePath, "//"), "/", 2)
			if len(parts) == 2 {
				uri.Host = parts[0]
				tail := strings.TrimPrefix(uriPath, volumePath)
				uriPath = "/" + parts[1] + tail
			}
		}
	}
	uri.Path = uriPath
	return uri.String(), nil
}
