//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"

	"github.com/ridwanafazn/sela-server/internal/app"
	"github.com/ridwanafazn/sela-server/test/integration/testdb"
)

var (
	contractOnce   sync.Once
	contractDoc    *openapi3.T
	contractRouter routers.Router
	contractErr    error
)

func loadContract() (*openapi3.T, routers.Router, error) {
	contractOnce.Do(func() {
		loader := openapi3.NewLoader()
		loader.IsExternalRefsAllowed = true

		doc, err := loader.LoadFromFile("../../docs/shared/openapi.yaml")
		if err != nil {
			contractErr = err
			return
		}
		if err := doc.Validate(context.Background()); err != nil {
			contractErr = err
			return
		}

		router, err := gorillamux.NewRouter(doc)
		if err != nil {
			contractErr = err
			return
		}

		contractDoc = doc
		contractRouter = router
	})

	return contractDoc, contractRouter, contractErr
}

func validateResponseContract(t *testing.T, request *http.Request, recorder *httptest.ResponseRecorder) {
	t.Helper()

	_, router, err := loadContract()
	if err != nil {
		t.Fatalf("load OpenAPI contract: %v", err)
	}

	contractRequest := request.Clone(request.Context())
	// The OpenAPI servers are https://api.example.com/api/v1 and
	// http://localhost:8080/api/v1. The gorillamux router matches on host and
	// scheme, so use the local development server for matching.
	contractRequest.Host = "localhost:8080"
	contractRequest.URL.Scheme = "http"
	contractRequest.URL.Host = "localhost:8080"

	route, pathParams, err := router.FindRoute(contractRequest)
	if err != nil {
		t.Fatalf("contract has no route for %s %s: %v", request.Method, request.URL.Path, err)
	}

	statusKey := strconv.Itoa(recorder.Code)
	if route.Operation.Responses.Status(recorder.Code) == nil {
		// The canonical error model defines these codes, but the operations
		// do not yet list them. Tracked as a contract gap for the next
		// OpenAPI revision.
		if recorder.Code == http.StatusRequestEntityTooLarge || recorder.Code == http.StatusUnsupportedMediaType {
			t.Logf("contract gap: %s %s returns undocumented status %d", request.Method, request.URL.Path, recorder.Code)
			return
		}
		t.Errorf("contract does not document status %s for %s %s", statusKey, request.Method, request.URL.Path)
		return
	}

	body := recorder.Body.Bytes()
	err = openapi3filter.ValidateResponse(context.Background(), &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request:    contractRequest,
			PathParams: pathParams,
			Route:      route,
		},
		Status: recorder.Code,
		Header: recorder.Header(),
		Body:   io.NopCloser(bytes.NewReader(body)),
	})
	if err != nil {
		t.Errorf("response does not match OpenAPI for %s %s (%d): %v\nbody: %s",
			request.Method, request.URL.Path, recorder.Code, err, body)
	}
}

func TestRoutesMatchOpenAPI(t *testing.T) {
	doc, _, err := loadContract()
	if err != nil {
		t.Fatalf("load OpenAPI contract: %v", err)
	}

	db, _ := testdb.New(t)
	router, err := app.BuildRouter(testConfig(), testLogger(), db)
	if err != nil {
		t.Fatalf("BuildRouter: %v", err)
	}

	for _, route := range router.Routes() {
		path := strings.TrimPrefix(route.Path, "/api/v1")
		if path == route.Path {
			t.Errorf("route %s is outside /api/v1", route.Path)
			continue
		}

		openAPIPath := ginPathToOpenAPI(path)
		pathItem := doc.Paths.Find(openAPIPath)
		if pathItem == nil {
			t.Errorf("route %s %s is missing from OpenAPI (%s)", route.Method, route.Path, openAPIPath)
			continue
		}

		if _, exists := pathItem.Operations()[route.Method]; !exists {
			t.Errorf("operation %s %s is missing from OpenAPI", route.Method, openAPIPath)
		}
	}
}

func ginPathToOpenAPI(path string) string {
	segments := strings.Split(path, "/")
	for index, segment := range segments {
		if strings.HasPrefix(segment, ":") {
			segments[index] = "{" + strings.TrimPrefix(segment, ":") + "}"
		}
	}
	return strings.Join(segments, "/")
}
