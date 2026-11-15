//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type journalPayload struct {
	ID        string    `json:"id"`
	Title     *string   `json:"title"`
	Content   string    `json:"content"`
	EntryDate string    `json:"entry_date"`
	Status    string    `json:"status"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type journalEnvelope struct {
	Data  journalPayload `json:"data"`
	Error errorPayload   `json:"error"`
}

type journalListEnvelope struct {
	Data       []journalPayload `json:"data"`
	Pagination struct {
		NextCursor *string `json:"next_cursor"`
		HasMore    bool    `json:"has_more"`
	} `json:"pagination"`
	Error errorPayload `json:"error"`
}

func newTestUser(t *testing.T, router *gin.Engine) authPayload {
	t.Helper()

	email := "journal-" + uuid.NewString() + "@example.com"
	result := register(t, router, email, testPassword, "", "203.0.113.200")
	if result.Status != http.StatusCreated {
		t.Fatalf("register test user = %d: %s", result.Status, result.Body)
	}

	return decodeJSON[authEnvelope](t, result.Body).Data
}

func createJournalAPI(t *testing.T, router *gin.Engine, token, idempotencyKey string, body map[string]any) apiResult {
	t.Helper()

	headers := map[string]string{}
	if idempotencyKey != "" {
		headers["Idempotency-Key"] = idempotencyKey
	}

	return do(t, router, http.MethodPost, "/api/v1/journals", requestOptions{
		body:    body,
		token:   token,
		ip:      "203.0.113.201",
		headers: headers,
	})
}

func listJournalsAPI(t *testing.T, router *gin.Engine, token, query string) apiResult {
	t.Helper()

	path := "/api/v1/journals"
	if query != "" {
		path += "?" + query
	}

	return do(t, router, http.MethodGet, path, requestOptions{
		token: token,
		ip:    "203.0.113.201",
	})
}

func journalAPIPath(journalID string) string {
	return "/api/v1/journals/" + journalID
}

func mustCreateJournal(t *testing.T, router *gin.Engine, token string, body map[string]any) journalPayload {
	t.Helper()

	result := createJournalAPI(t, router, token, uuid.NewString(), body)
	if result.Status != http.StatusCreated {
		t.Fatalf("create journal = %d: %s", result.Status, result.Body)
	}
	return decodeJSON[journalEnvelope](t, result.Body).Data
}

func TestJournalWriteRateLimit(t *testing.T) {
	cfg := testConfig()
	cfg.RateLimit.JournalPerMinute = 1
	router, _ := newAPI(t, cfg)
	user := newTestUser(t, router)

	first := createJournalAPI(t, router, user.AccessToken, uuid.NewString(), map[string]any{
		"content":    "First write.",
		"entry_date": "2026-11-10",
	})
	if first.Status != http.StatusCreated {
		t.Fatalf("first journal = %d: %s", first.Status, first.Body)
	}

	second := createJournalAPI(t, router, user.AccessToken, uuid.NewString(), map[string]any{
		"content":    "Second write.",
		"entry_date": "2026-11-11",
	})
	if second.Status != http.StatusTooManyRequests {
		t.Fatalf("second journal = %d, want 429: %s", second.Status, second.Body)
	}
	if second.Header.Get("Retry-After") == "" {
		t.Error("429 response is missing Retry-After")
	}
}

func TestJournalCreateIdempotency(t *testing.T) {
	router, _ := newAPI(t, testConfig())
	user := newTestUser(t, router)

	key := uuid.NewString()
	body := map[string]any{
		"title":      "First entry",
		"content":    "I could not sleep well.",
		"entry_date": "2026-11-01",
		"status":     "draft",
	}

	first := createJournalAPI(t, router, user.AccessToken, key, body)
	if first.Status != http.StatusCreated {
		t.Fatalf("create = %d, want 201: %s", first.Status, first.Body)
	}
	created := decodeJSON[journalEnvelope](t, first.Body).Data
	if created.Version != 1 || created.Status != "draft" || created.EntryDate != "2026-11-01" {
		t.Errorf("created = %+v", created)
	}
	if created.Title == nil || *created.Title != "First entry" {
		t.Errorf("title = %v, want First entry", created.Title)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Error("timestamps are zero")
	}

	replay := createJournalAPI(t, router, user.AccessToken, key, body)
	if replay.Status != http.StatusCreated {
		t.Fatalf("replay = %d, want 201: %s", replay.Status, replay.Body)
	}
	replayed := decodeJSON[journalEnvelope](t, replay.Body).Data
	if replayed.ID != created.ID {
		t.Errorf("replay created a new journal: %s != %s", replayed.ID, created.ID)
	}
	if replayed.Version != created.Version {
		t.Errorf("replay version = %d, want %d", replayed.Version, created.Version)
	}

	conflict := createJournalAPI(t, router, user.AccessToken, key, map[string]any{
		"content": "different request body",
	})
	if conflict.Status != http.StatusConflict {
		t.Fatalf("reused key = %d, want 409: %s", conflict.Status, conflict.Body)
	}
	if envelope := decodeJSON[errorEnvelope](t, conflict.Body); envelope.Error.Code != "IDEMPOTENCY_KEY_REUSED" {
		t.Errorf("error code = %q, want IDEMPOTENCY_KEY_REUSED", envelope.Error.Code)
	}

	missing := createJournalAPI(t, router, user.AccessToken, "", body)
	if missing.Status != http.StatusBadRequest {
		t.Fatalf("missing key = %d, want 400: %s", missing.Status, missing.Body)
	}
	if envelope := decodeJSON[errorEnvelope](t, missing.Body); !hasField(envelope.Error, "Idempotency-Key") {
		t.Errorf("details = %+v, want Idempotency-Key", envelope.Error.Details)
	}

	malformed := createJournalAPI(t, router, user.AccessToken, "not-a-uuid", body)
	if malformed.Status != http.StatusBadRequest {
		t.Fatalf("malformed key = %d, want 400: %s", malformed.Status, malformed.Body)
	}

	otherUser := newTestUser(t, router)
	scoped := createJournalAPI(t, router, otherUser.AccessToken, key, map[string]any{"content": "same key, other user"})
	if scoped.Status != http.StatusCreated {
		t.Fatalf("same key for another user = %d, want 201: %s", scoped.Status, scoped.Body)
	}
	scopedJournal := decodeJSON[journalEnvelope](t, scoped.Body).Data
	if scopedJournal.ID == created.ID {
		t.Error("idempotency key was not scoped per user")
	}
}

func TestJournalCreateDefaultsAndShape(t *testing.T) {
	router, _ := newAPI(t, testConfig())
	user := newTestUser(t, router)

	result := createJournalAPI(t, router, user.AccessToken, uuid.NewString(), map[string]any{
		"content": "Just a thought.",
	})
	if result.Status != http.StatusCreated {
		t.Fatalf("create = %d, want 201: %s", result.Status, result.Body)
	}

	envelope := decodeJSON[journalEnvelope](t, result.Body)
	created := envelope.Data
	if created.Title != nil {
		t.Errorf("title = %v, want null", created.Title)
	}
	if created.Status != "active" {
		t.Errorf("status = %q, want active", created.Status)
	}
	if _, err := time.Parse("2006-01-02", created.EntryDate); err != nil {
		t.Errorf("entry_date = %q is not a date", created.EntryDate)
	}
	if created.Version != 1 {
		t.Errorf("version = %d, want 1", created.Version)
	}

	var shape map[string]any
	if err := json.Unmarshal(result.Body, &shape); err != nil {
		t.Fatalf("decode shape: %v", err)
	}
	data, ok := shape["data"].(map[string]any)
	if !ok {
		t.Fatalf("data is %T, want object", shape["data"])
	}
	if _, exists := data["user_id"]; exists {
		t.Error("response exposes user_id")
	}
	if _, exists := data["deleted_at"]; exists {
		t.Error("response exposes deleted_at")
	}
}

func TestJournalListPaginationAndFilters(t *testing.T) {
	router, _ := newAPI(t, testConfig())
	user := newTestUser(t, router)

	mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"content": "first", "entry_date": "2026-11-01", "status": "active",
	})
	mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"title": "Draft sleep notes", "content": "sleep notes", "entry_date": "2026-11-02", "status": "draft",
	})
	mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"content": "third", "entry_date": "2026-11-03", "status": "active",
	})
	mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"content": "fourth", "entry_date": "2026-11-04", "status": "active",
	})

	firstPage := listJournalsAPI(t, router, user.AccessToken, "limit=2")
	if firstPage.Status != http.StatusOK {
		t.Fatalf("list = %d: %s", firstPage.Status, firstPage.Body)
	}
	pageOne := decodeJSON[journalListEnvelope](t, firstPage.Body)
	if len(pageOne.Data) != 2 {
		t.Fatalf("page one size = %d, want 2", len(pageOne.Data))
	}
	if pageOne.Data[0].EntryDate != "2026-11-04" || pageOne.Data[1].EntryDate != "2026-11-03" {
		t.Errorf("page one order = %s, %s", pageOne.Data[0].EntryDate, pageOne.Data[1].EntryDate)
	}
	if !pageOne.Pagination.HasMore || pageOne.Pagination.NextCursor == nil {
		t.Fatal("page one is missing pagination metadata")
	}

	secondPage := listJournalsAPI(t, router, user.AccessToken,
		"limit=2&cursor="+url.QueryEscape(*pageOne.Pagination.NextCursor))
	if secondPage.Status != http.StatusOK {
		t.Fatalf("page two = %d: %s", secondPage.Status, secondPage.Body)
	}
	pageTwo := decodeJSON[journalListEnvelope](t, secondPage.Body)
	if len(pageTwo.Data) != 2 {
		t.Fatalf("page two size = %d, want 2", len(pageTwo.Data))
	}
	if pageTwo.Data[0].EntryDate != "2026-11-02" || pageTwo.Data[1].EntryDate != "2026-11-01" {
		t.Errorf("page two order = %s, %s", pageTwo.Data[0].EntryDate, pageTwo.Data[1].EntryDate)
	}
	if pageTwo.Pagination.HasMore || pageTwo.Pagination.NextCursor != nil {
		t.Error("page two should be the final page")
	}

	drafts := decodeJSON[journalListEnvelope](t, listJournalsAPI(t, router, user.AccessToken, "status=draft").Body)
	if len(drafts.Data) != 1 || drafts.Data[0].Status != "draft" {
		t.Errorf("draft filter = %+v", drafts.Data)
	}

	active := decodeJSON[journalListEnvelope](t, listJournalsAPI(t, router, user.AccessToken, "status=active").Body)
	if len(active.Data) != 3 {
		t.Errorf("active filter size = %d, want 3", len(active.Data))
	}

	rangePage := decodeJSON[journalListEnvelope](t, listJournalsAPI(t, router, user.AccessToken, "from=2026-11-02&to=2026-11-03").Body)
	if len(rangePage.Data) != 2 || rangePage.Data[0].EntryDate != "2026-11-03" || rangePage.Data[1].EntryDate != "2026-11-02" {
		t.Errorf("date range = %+v", rangePage.Data)
	}

	search := decodeJSON[journalListEnvelope](t, listJournalsAPI(t, router, user.AccessToken, "q=sleep").Body)
	if len(search.Data) != 1 || search.Data[0].Status != "draft" {
		t.Errorf("search = %+v, want the owner's draft", search.Data)
	}

	for _, query := range []string{
		"cursor=not-a-cursor",
		"limit=101",
		"limit=0",
		"status=published",
		"from=2026-11-05&to=2026-11-01",
		"q=" + url.QueryEscape(strings.Repeat("a", 201)),
	} {
		result := listJournalsAPI(t, router, user.AccessToken, query)
		if result.Status != http.StatusBadRequest {
			t.Errorf("query %q = %d, want 400: %s", query, result.Status, result.Body)
		}
	}
}

func TestJournalPaginationWithEqualDates(t *testing.T) {
	router, _ := newAPI(t, testConfig())
	user := newTestUser(t, router)

	expected := map[string]bool{}
	for index := 0; index < 3; index++ {
		created := mustCreateJournal(t, router, user.AccessToken, map[string]any{
			"content": "same date entry", "entry_date": "2026-11-06",
		})
		expected[created.ID] = true
	}

	seen := map[string]bool{}
	query := "limit=2"
	for page := 0; page < 3; page++ {
		result := listJournalsAPI(t, router, user.AccessToken, query)
		if result.Status != http.StatusOK {
			t.Fatalf("page %d = %d: %s", page, result.Status, result.Body)
		}

		body := decodeJSON[journalListEnvelope](t, result.Body)
		for _, item := range body.Data {
			if seen[item.ID] {
				t.Errorf("journal %s appeared on more than one page", item.ID)
			}
			seen[item.ID] = true
		}

		if !body.Pagination.HasMore {
			break
		}
		if body.Pagination.NextCursor == nil {
			t.Fatal("has_more is true without a cursor")
		}
		query = "limit=2&cursor=" + url.QueryEscape(*body.Pagination.NextCursor)
	}

	if len(seen) != len(expected) {
		t.Errorf("saw %d journals across pages, want %d", len(seen), len(expected))
	}
	for id := range expected {
		if !seen[id] {
			t.Errorf("journal %s was skipped", id)
		}
	}
}

func TestJournalUpdateIfMatch(t *testing.T) {
	router, _ := newAPI(t, testConfig())
	user := newTestUser(t, router)
	created := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"title": "Original", "content": "original content", "entry_date": "2026-11-01",
	})

	missing := do(t, router, http.MethodPatch, journalAPIPath(created.ID), requestOptions{
		token: user.AccessToken,
		body:  map[string]any{"content": "updated"},
	})
	if missing.Status != http.StatusPreconditionRequired {
		t.Fatalf("missing If-Match = %d, want 428: %s", missing.Status, missing.Body)
	}

	updated := do(t, router, http.MethodPatch, journalAPIPath(created.ID), requestOptions{
		token:   user.AccessToken,
		headers: map[string]string{"If-Match": "1"},
		body: map[string]any{
			"title":      nil,
			"content":    "updated content",
			"entry_date": "2026-11-05",
			"status":     "draft",
		},
	})
	if updated.Status != http.StatusOK {
		t.Fatalf("update = %d, want 200: %s", updated.Status, updated.Body)
	}
	updatedJournal := decodeJSON[journalEnvelope](t, updated.Body).Data
	if updatedJournal.Version != 2 {
		t.Errorf("version = %d, want 2", updatedJournal.Version)
	}
	if updatedJournal.Title != nil {
		t.Errorf("title = %v, want null", updatedJournal.Title)
	}
	if updatedJournal.Content != "updated content" || updatedJournal.EntryDate != "2026-11-05" || updatedJournal.Status != "draft" {
		t.Errorf("updated = %+v", updatedJournal)
	}

	stale := do(t, router, http.MethodPatch, journalAPIPath(created.ID), requestOptions{
		token:   user.AccessToken,
		headers: map[string]string{"If-Match": "1"},
		body:    map[string]any{"content": "stale"},
	})
	if stale.Status != http.StatusConflict {
		t.Fatalf("stale update = %d, want 409: %s", stale.Status, stale.Body)
	}
	if envelope := decodeJSON[errorEnvelope](t, stale.Body); envelope.Error.Code != "VERSION_CONFLICT" {
		t.Errorf("error code = %q, want VERSION_CONFLICT", envelope.Error.Code)
	}

	noop := do(t, router, http.MethodPatch, journalAPIPath(created.ID), requestOptions{
		token:   user.AccessToken,
		headers: map[string]string{"If-Match": "2"},
		body:    map[string]any{},
	})
	if noop.Status != http.StatusOK {
		t.Fatalf("empty patch = %d, want 200: %s", noop.Status, noop.Body)
	}
	if noopJournal := decodeJSON[journalEnvelope](t, noop.Body).Data; noopJournal.Version != 2 {
		t.Errorf("empty patch version = %d, want 2", noopJournal.Version)
	}

	invalidStatus := do(t, router, http.MethodPatch, journalAPIPath(created.ID), requestOptions{
		token:   user.AccessToken,
		headers: map[string]string{"If-Match": "2"},
		body:    map[string]any{"status": "published"},
	})
	if invalidStatus.Status != http.StatusBadRequest {
		t.Fatalf("invalid status = %d, want 400: %s", invalidStatus.Status, invalidStatus.Body)
	}

	invalidHeader := do(t, router, http.MethodPatch, journalAPIPath(created.ID), requestOptions{
		token:   user.AccessToken,
		headers: map[string]string{"If-Match": "abc"},
		body:    map[string]any{"content": "value"},
	})
	if invalidHeader.Status != http.StatusBadRequest {
		t.Fatalf("invalid If-Match = %d, want 400: %s", invalidHeader.Status, invalidHeader.Body)
	}

	invalidID := do(t, router, http.MethodPatch, journalAPIPath("not-a-uuid"), requestOptions{
		token:   user.AccessToken,
		headers: map[string]string{"If-Match": "1"},
		body:    map[string]any{"content": "value"},
	})
	if invalidID.Status != http.StatusNotFound {
		t.Fatalf("invalid id = %d, want 404", invalidID.Status)
	}
}

func TestJournalDelete(t *testing.T) {
	router, db := newAPI(t, testConfig())
	user := newTestUser(t, router)
	created := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"content": "to delete", "entry_date": "2026-11-01",
	})

	missing := do(t, router, http.MethodDelete, journalAPIPath(created.ID), requestOptions{token: user.AccessToken})
	if missing.Status != http.StatusPreconditionRequired {
		t.Fatalf("missing If-Match = %d, want 428", missing.Status)
	}

	stale := do(t, router, http.MethodDelete, journalAPIPath(created.ID), requestOptions{
		token:   user.AccessToken,
		headers: map[string]string{"If-Match": "9"},
	})
	if stale.Status != http.StatusConflict {
		t.Fatalf("stale delete = %d, want 409: %s", stale.Status, stale.Body)
	}

	deleted := do(t, router, http.MethodDelete, journalAPIPath(created.ID), requestOptions{
		token:   user.AccessToken,
		headers: map[string]string{"If-Match": "1"},
	})
	if deleted.Status != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204: %s", deleted.Status, deleted.Body)
	}

	detail := do(t, router, http.MethodGet, journalAPIPath(created.ID), requestOptions{token: user.AccessToken})
	if detail.Status != http.StatusNotFound {
		t.Fatalf("get after delete = %d, want 404", detail.Status)
	}

	list := decodeJSON[journalListEnvelope](t, listJournalsAPI(t, router, user.AccessToken, "").Body)
	if len(list.Data) != 0 {
		t.Errorf("list after delete contains %d journals", len(list.Data))
	}

	var (
		version   int
		deletedAt *time.Time
	)
	if err := db.SQL.QueryRowContext(context.Background(),
		`SELECT version, deleted_at FROM journals WHERE id = $1`, created.ID,
	).Scan(&version, &deletedAt); err != nil {
		t.Fatalf("load tombstone: %v", err)
	}
	if version != 2 {
		t.Errorf("tombstone version = %d, want 2", version)
	}
	if deletedAt == nil {
		t.Error("tombstone deleted_at is null")
	}

	again := do(t, router, http.MethodDelete, journalAPIPath(created.ID), requestOptions{
		token:   user.AccessToken,
		headers: map[string]string{"If-Match": "2"},
	})
	if again.Status != http.StatusNotFound {
		t.Fatalf("delete again = %d, want 404", again.Status)
	}
}

func TestJournalOwnership(t *testing.T) {
	router, _ := newAPI(t, testConfig())
	owner := newTestUser(t, router)
	stranger := newTestUser(t, router)

	created := mustCreateJournal(t, router, owner.AccessToken, map[string]any{
		"content": "private entry", "entry_date": "2026-11-02",
	})

	if result := do(t, router, http.MethodGet, journalAPIPath(created.ID), requestOptions{token: stranger.AccessToken}); result.Status != http.StatusNotFound {
		t.Errorf("stranger get = %d, want 404", result.Status)
	}
	if result := do(t, router, http.MethodPatch, journalAPIPath(created.ID), requestOptions{
		token:   stranger.AccessToken,
		headers: map[string]string{"If-Match": "1"},
		body:    map[string]any{"content": "stolen"},
	}); result.Status != http.StatusNotFound {
		t.Errorf("stranger update = %d, want 404", result.Status)
	}
	if result := do(t, router, http.MethodDelete, journalAPIPath(created.ID), requestOptions{
		token:   stranger.AccessToken,
		headers: map[string]string{"If-Match": "1"},
	}); result.Status != http.StatusNotFound {
		t.Errorf("stranger delete = %d, want 404", result.Status)
	}

	strangerList := decodeJSON[journalListEnvelope](t, listJournalsAPI(t, router, stranger.AccessToken, "").Body)
	if len(strangerList.Data) != 0 {
		t.Errorf("stranger list contains %d journals", len(strangerList.Data))
	}

	ownerDetail := do(t, router, http.MethodGet, journalAPIPath(created.ID), requestOptions{token: owner.AccessToken})
	if ownerDetail.Status != http.StatusOK {
		t.Fatalf("owner get = %d, want 200", ownerDetail.Status)
	}
}

func TestJournalValidation(t *testing.T) {
	router, _ := newAPI(t, testConfig())
	user := newTestUser(t, router)

	tests := []struct {
		name  string
		body  map[string]any
		field string
	}{
		{name: "empty content", body: map[string]any{"content": ""}, field: "content"},
		{name: "long title", body: map[string]any{"content": "content", "title": strings.Repeat("a", 201)}, field: "title"},
		{name: "invalid date", body: map[string]any{"content": "content", "entry_date": "03-11-2026"}, field: "entry_date"},
		{name: "invalid status", body: map[string]any{"content": "content", "status": "published"}, field: "status"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := createJournalAPI(t, router, user.AccessToken, uuid.NewString(), test.body)
			if result.Status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", result.Status, result.Body)
			}
			envelope := decodeJSON[errorEnvelope](t, result.Body)
			if !hasField(envelope.Error, test.field) {
				t.Errorf("details = %+v, want field %s", envelope.Error.Details, test.field)
			}
		})
	}
}

func hasField(payload errorPayload, field string) bool {
	for _, detail := range payload.Details {
		if detail.Field == field {
			return true
		}
	}
	return false
}
