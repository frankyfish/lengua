package lengua

import (
	"encoding/xml"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/frankyfish/lengua/lsd"
)

// DictionaryInfo describes a dictionary found by [Open]. It is available
// without opening (decoding) the dictionary.
type DictionaryInfo struct {
	Name         string       // e.g. "LingvoUniversal (En-Ru)"
	Source       lsd.Language // language of the headings
	Target       lsd.Language // language of the articles
	HeadingCount int          // number of headings
	Path         string       // the .lsd file
	AbbrevPath   string       // the matching *_abrv.lsd file, if any
	SoundPath    string       // the .lsa sound archive, if any
	ID           string       // ABBYY product id from the .lod file, if any
}

// lodFile is the XML metadata file shipped next to each dictionary.
type lodFile struct {
	ID        string `xml:"id"`
	Name      string `xml:"name"`
	SoundName string `xml:"soundName"`
}

// scan is the result of walking an installation directory.
type scan struct {
	lsd      []string          // main dictionaries
	abbrev   map[string]string // lower-cased main file path without ext -> *_abrv.lsd
	global   string            // shared Abbrev.lsd
	lods     map[string]lodFile
	archives map[string]string // lower-cased base name without ext -> .lsa path
}

func scanDir(root string) (*scan, error) {
	s := &scan{
		abbrev:   map[string]string{},
		lods:     map[string]lodFile{},
		archives: map[string]string{},
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		name := d.Name()
		ext := strings.ToLower(filepath.Ext(name))
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		switch {
		case ext == ".lsd" && strings.EqualFold(stem, "abbrev"):
			if s.global == "" {
				s.global = path
			}
		case ext == ".lsd" && strings.HasSuffix(strings.ToLower(stem), "_abrv"):
			main := strings.TrimSuffix(path, filepath.Ext(path))
			main = main[:len(main)-len("_abrv")]
			s.abbrev[strings.ToLower(main)] = path
		case ext == ".lsd":
			s.lsd = append(s.lsd, path)
		case ext == ".lod":
			if lod, err := readLOD(path); err == nil && lod.Name != "" {
				s.lods[lod.Name] = lod
			}
		case ext == ".lsa":
			s.archives[strings.ToLower(stem)] = path
		}
		return nil
	})
	sort.Strings(s.lsd)
	return s, err
}

func readLOD(path string) (lodFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return lodFile{}, err
	}
	var lod lodFile
	err = xml.Unmarshal(data, &lod)
	return lod, err
}

// catalog builds the dictionary list from a scan. Files that are not
// readable X3 dictionaries are reported in skipped.
func (s *scan) catalog() (infos []DictionaryInfo, skipped map[string]error) {
	skipped = map[string]error{}
	seen := map[string]bool{}
	for _, path := range s.lsd {
		hdr, err := lsd.ReadInfo(path)
		if err != nil {
			skipped[path] = err
			continue
		}
		if seen[hdr.Name] {
			continue // the same dictionary installed twice
		}
		seen[hdr.Name] = true
		info := DictionaryInfo{
			Name:         hdr.Name,
			Source:       hdr.Source,
			Target:       hdr.Target,
			HeadingCount: hdr.HeadingCount,
			Path:         path,
			AbbrevPath:   s.abbrev[strings.ToLower(strings.TrimSuffix(path, filepath.Ext(path)))],
		}
		if lod, ok := s.lods[hdr.Name]; ok {
			info.ID = lod.ID
			if lod.SoundName != "" {
				info.SoundPath = s.archives[strings.ToLower(lod.SoundName)]
			}
		}
		infos = append(infos, info)
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })
	return infos, skipped
}

// fileStem returns the file name without directory and extension.
func fileStem(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
