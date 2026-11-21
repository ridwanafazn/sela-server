//go:build integration

package integration_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

type journalChangePayload struct {
	ID        string          `json:"id"`
	Version   int             `json:"version"`
	UpdatedAt time.Time       `json:"updated_at"`
	DeletedAt *time.Time      `json:"deleted_at"`
	Journal   *journalPayload `json:"journal"`
}

type journalChangeListPayload struct {
	Data       []journalChangePayload `json:"data"`
	Pagination struct {
		NextCursor *string `json:"next_cursor"`
		HasMore    bool    `json:"has_more"`
	} `json:"pagination"`
	Sync struct {
		ServerTime time.Time `json:"server_time"`
	} `json:"sync"`
}

func TestJournalChangesPaginationAndTombstones(t *testing.T) {
	router, _ := newAPI(t, testConfig())
	user := newTestUser(t, router)
	stranger := newTestUser(t, router)

	active := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"title":      "Active entry",
		"content":    "The first version of this entry.",
		"entry_date": "2026-11-10",
	})
	draft := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"title":      "Draft entry",
		"content":    "A draft that must still sync.",
		"entry_date": "2026-11-11",
		"status":     "draft",
	})
	other := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"title":      "Other entry",
		"content":    "A third record for pagination.",
		"entry_date": "2026-11-12",
	})

	updated := do(t, router, http.MethodPatch, journalAPIPath(active.ID), requestOptions{
		token:   user.AccessToken,
		headers: map[string]string{"If-Match": "1"},
		body:    map[string]any{"content": "The updated version of this entry."},
	})
	if updated.Status != http.StatusOK {
		t.Fatalf("update active journal = %d: %s", updated.Status, updated.Body)
	}

	deleted := do(t, router, http.MethodDelete, journalAPIPath(draft.ID), requestOptions{
		token:   user.AccessToken,
		headers: map[string]string{"If-Match": "1"},
	})
	if deleted.Status != http.StatusNoContent {
		t.Fatalf("delete draft journal = %d: %s", deleted.Status, deleted.Body)
	}

	first := do(t, router, http.MethodGet, "/api/v1/journal-changes?limit=2", requestOptions{
		token: user.AccessToken,
	})
	if first.Status != http.StatusOK {
		t.Fatalf("journal changes first page = %d: %s", first.Status, first.Body)
	}
	firstPage := decodeJSON[journalChangeListPayload](t, first.Body)
	if len(firstPage.Data) != 2 || !firstPage.Pagination.HasMore || firstPage.Pagination.NextCursor == nil {
		t.Fatalf("first page = %+v", firstPage)
	}
	if firstPage.Sync.ServerTime.IsZero() {
		t.Error("first page is missing sync.server_time")
	}
	for index := 1; index < len(firstPage.Data); index++ {
		if firstPage.Data[index].UpdatedAt.Before(firstPage.Data[index-1].UpdatedAt) {
			t.Errorf("first page is not ordered by updated_at: %+v", firstPage.Data)
		}
	}

	second := do(t, router, http.MethodGet,
		"/api/v1/journal-changes?limit=2&cursor="+*firstPage.Pagination.NextCursor,
		requestOptions{token: user.AccessToken},
	)
	if second.Status != http.StatusOK {
		t.Fatalf("journal changes second page = %d: %s", second.Status, second.Body)
	}
	secondPage := decodeJSON[journalChangeListPayload](t, second.Body)
	if len(secondPage.Data) != 1 || secondPage.Pagination.HasMore {
		t.Fatalf("second page = %+v", secondPage)
	}
	if secondPage.Data[0].UpdatedAt.Before(firstPage.Data[len(firstPage.Data)-1].UpdatedAt) {
		t.Errorf("second page is out of order: %+v", secondPage.Data)
	}

	changes := append(firstPage.Data, secondPage.Data...)
	if len(changes) != 3 {
		t.Fatalf("changes = %d, want 3", len(changes))
	}
	byID := make(map[string]journalChangePayload, len(changes))
	for _, change := range changes {
		if _, exists := byID[change.ID]; exists {
			t.Errorf("duplicate change for %s", change.ID)
		}
		byID[change.ID] = change
	}

	activeChange, ok := byID[active.ID]
	if !ok {
		t.Fatal("updated active journal is missing from changes")
	}
	if activeChange.Journal == nil || activeChange.DeletedAt != nil {
		t.Errorf("active change = %+v, want a journal and no tombstone", activeChange)
	}
	if activeChange.Version != 2 {
		t.Errorf("active version = %d, want 2", activeChange.Version)
	}
	if activeChange.Journal != nil && activeChange.Journal.Content != "The updated version of this entry." {
		t.Errorf("active content = %q", activeChange.Journal.Content)
	}

	tombstone, ok := byID[draft.ID]
	if !ok {
		t.Fatal("deleted draft is missing from changes")
	}
	if tombstone.Journal != nil {
		t.Errorf("tombstone journal = %+v, want null", tombstone.Journal)
	}
	if tombstone.DeletedAt == nil || tombstone.Version != 2 {
		t.Errorf("tombstone = %+v, want deleted_at and version 2", tombstone)
	}

	otherChange, ok := byID[other.ID]
	if !ok {
		t.Fatal("third journal is missing from changes")
	}
	if otherChange.Journal == nil || otherChange.Journal.Status != "active" {
		t.Errorf("third change = %+v", otherChange)
	}

	strangerChanges := decodeJSON[journalChangeListPayload](t, do(t, router, http.MethodGet,
		"/api/v1/journal-changes", requestOptions{token: stranger.AccessToken}).Body)
	if len(strangerChanges.Data) != 0 {
		t.Errorf("stranger changes = %d, want 0", len(strangerChanges.Data))
	}

	invalid := do(t, router, http.MethodGet,
		"/api/v1/journal-changes?cursor="+uuid.NewString(),
		requestOptions{token: user.AccessToken},
	)
	if invalid.Status != http.StatusBadRequest {
		t.Errorf("invalid cursor = %d, want 400", invalid.Status)
	}
}
