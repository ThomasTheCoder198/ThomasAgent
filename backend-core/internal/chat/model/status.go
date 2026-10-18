package model

import "github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"

// StatusKey names a data-status label; labels are localized here because run-view renders them verbatim (B2-9).
type StatusKey string

const (
	StatusSearchingKB       StatusKey = "searching_kb"
	StatusReadingKB         StatusKey = "reading_kb"
	StatusRetryingSearch    StatusKey = "retrying_search"
	StatusKBDeleted         StatusKey = "kb_deleted"
	StatusCheckingCitations StatusKey = "checking_citations"
	StatusCompacting        StatusKey = "compacting"
	StatusSteered           StatusKey = "steered"
)

var statusLabels = map[StatusKey][2]string{ // [vi, en]
	StatusSearchingKB:       {"Đang tìm trong KB…", "Searching the KB…"},
	StatusReadingKB:         {"Đang đọc tài liệu…", "Reading the document…"},
	StatusRetryingSearch:    {"Đang tìm lại với câu hỏi khác…", "Searching again with another query…"},
	StatusKBDeleted:         {"KB đã bị xoá", "The KB was deleted"},
	StatusCheckingCitations: {"Đang kiểm tra trích dẫn…", "Checking citations…"},
	StatusCompacting:        {"Đang tóm tắt hội thoại cũ…", "Summarizing earlier messages…"},
	StatusSteered:           {"Đã nhận chỉ dẫn mới", "New instruction received"},
}

func StatusLabel(key StatusKey, lang errors.Lang) string {
	labels := statusLabels[key]
	if lang == errors.LangEN {
		return labels[1]
	}
	return labels[0]
}
