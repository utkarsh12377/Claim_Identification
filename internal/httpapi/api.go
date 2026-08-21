package httpapi

import (
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/utkarsh/claim-identification/internal/store"
	"github.com/utkarsh/claim-identification/internal/workflow"
)

type API struct {
	engine   *workflow.Engine
	store    store.Store
	log      *slog.Logger
	mediaDir string
}

type Config struct {
	Engine   *workflow.Engine
	Store    store.Store
	Logger   *slog.Logger
	MediaDir string
}

func New(cfg Config) *API {
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &API{
		engine:   cfg.Engine,
		store:    cfg.Store,
		log:      log,
		mediaDir: cfg.MediaDir,
	}
}

func (a *API) Handler() http.Handler {
	r := newRouter()

	r.handle(http.MethodPost, "/claims/identify", a.handleIdentify)
	r.handle(http.MethodGet, "/claims/status/{workflowId}", a.handleStatus)

	r.handle(http.MethodGet, "/claims/categories", a.handleCategories)
	r.handle(http.MethodGet, "/claims/rules", a.handleRules)
	r.handle(http.MethodGet, "/products/{productId}", a.handleGetProduct)
	r.handle(http.MethodGet, "/healthz", a.handleHealth)

	if a.mediaDir != "" {
		r.mux.Handle("GET /media/", http.StripPrefix("/media/", http.FileServer(http.Dir(a.mediaDir))))
	}

	r.finalize(a.methodNotAllowed)
	r.mux.HandleFunc("/", a.handleNotFound)

	return withRequestID(withLogging(a.log, withRecovery(a.log, r.mux)))
}

type router struct {
	mux     *http.ServeMux
	methods map[string][]string
}

func newRouter() *router {
	return &router{mux: http.NewServeMux(), methods: make(map[string][]string)}
}

func (r *router) handle(method, pattern string, h http.HandlerFunc) {
	r.mux.HandleFunc(method+" "+pattern, h)
	r.methods[pattern] = append(r.methods[pattern], method)
}

func (r *router) finalize(onMethodNotAllowed func(allowed []string) http.HandlerFunc) {
	for pattern, methods := range r.methods {
		sort.Strings(methods)
		r.mux.HandleFunc(pattern, onMethodNotAllowed(methods))
	}
}

func (a *API) methodNotAllowed(allowed []string) http.HandlerFunc {
	allow := strings.Join(allowed, ", ")
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", allow)
		writeError(w, a.log, http.StatusMethodNotAllowed, codeInvalidRequest,
			r.Method+" is not allowed on "+r.URL.Path+", use "+allow)
	}
}

func (a *API) handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, a.log, http.StatusNotFound, codeInvalidRequest,
		"no route matches "+r.Method+" "+r.URL.Path)
}
