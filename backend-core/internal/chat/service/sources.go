package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/ragx"
)

const (
	sourceIDPrefix = "S"
	titleSeparator = " › "
	labelFormat    = "[%d.%d]"
	labelOpen      = "["
	labelClose     = "]"
	labelDot       = "."
)

type evidenceBlock struct{ Label, Text string }

// sourceRegistry numbers sources run-wide (R10) and keeps their blocks for the citation check (2b-8).
type sourceRegistry struct {
	mu      sync.Mutex
	byChunk map[string]int
	blocks  map[int][]evidenceBlock
	next    int
}

func newSourceRegistry() *sourceRegistry {
	return &sourceRegistry{byChunk: map[string]int{}, blocks: map[int][]evidenceBlock{}}
}

// number gives chunkID its run-wide number: the first one it was given, or the next one if it is new.
func (s *sourceRegistry) number(chunkID string) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n, ok := s.byChunk[chunkID]; ok {
		return n, false
	}
	s.next++
	s.byChunk[chunkID] = s.next
	return s.next, true
}

func (s *sourceRegistry) remember(n int, blocks []evidenceBlock) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blocks[n] = blocks
}

// present returns the tool data both the model and the tool part see (B2-2, index amendment 7): kb_search is
// renumbered run-wide and its new sources are cited as they appear; other tools pass through as valid JSON.
func (r *run) present(tool ragx.ToolName, data json.RawMessage) (json.RawMessage, error) {
	if tool == ragx.ToolKBSearch {
		return r.presentSearch(data)
	}
	if !json.Valid(data) {
		return nil, errors.ErrRagxUnavailable
	}
	return data, nil
}

func (r *run) presentSearch(data json.RawMessage) (json.RawMessage, error) {
	var out ragx.KbSearchOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, errors.ErrRagxUnavailable.WithCause(err)
	}
	for i := range out.Results {
		result := &out.Results[i]
		n, isNew := r.sources.number(result.ChunkId)
		result.Source = n
		blocks := make([]evidenceBlock, 0, len(result.Blocks))
		blockIDs := make([]string, 0, len(result.Blocks))
		locators := make([]json.RawMessage, 0, len(result.Blocks))
		for k := range result.Blocks {
			block := &result.Blocks[k]
			block.Label = fmt.Sprintf(labelFormat, n, blockIndex(block.Label, k))
			blocks = append(blocks, evidenceBlock{Label: block.Label, Text: block.Text})
			blockIDs = append(blockIDs, block.BlockId)
			locator, err := json.Marshal(block.Locator)
			if err != nil {
				return nil, err
			}
			locators = append(locators, locator)
		}
		if !isNew {
			continue
		}
		r.sources.remember(n, blocks)
		r.emit(model.SourceDocument{
			SourceID: sourceIDPrefix + strconv.Itoa(n), Title: sourceTitle(result.Section, result.FileName), FileName: result.FileName,
			Locator: model.CitationLocator{
				FileID: result.FileId, KBID: r.agent.KBID, FileName: result.FileName, FileVersion: result.FileVersion,
				KBName:  r.agent.KBName,
				Section: nonNil(result.Section), PageStart: result.PageStart, PageEnd: result.PageEnd, BlockIDs: blockIDs,
				Score: float64(result.Score), Locators: locators,
			},
		})
	}
	return json.Marshal(out)
}

// nonNil keeps a locator's section an array on the wire: the viewer joins it and a null would break it.
func nonNil(section []string) []string {
	if section == nil {
		return []string{}
	}
	return section
}

// blockIndex reads k from ragx's "[n.k]"; a label it cannot read keeps its position.
func blockIndex(label string, position int) int {
	inner := strings.TrimSuffix(strings.TrimPrefix(label, labelOpen), labelClose)
	if _, k, ok := strings.Cut(inner, labelDot); ok {
		if value, err := strconv.Atoi(k); err == nil {
			return value
		}
	}
	return position + 1
}

func sourceTitle(section []string, fileName string) string {
	if len(section) == 0 {
		return fileName
	}
	return strings.Join(section, titleSeparator)
}
