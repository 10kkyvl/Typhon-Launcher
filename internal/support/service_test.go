package support

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"typhon/internal/account"
	"typhon/internal/diagnostics"
	"typhon/internal/uierr"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var testInfo = ClientInfo{AppVersion: "9.9.9", OS: "windows", Arch: "amd64"}

type journal struct {
	mu     sync.Mutex
	events []string
}

func (j *journal) add(event string) {
	j.mu.Lock()
	j.events = append(j.events, event)
	j.mu.Unlock()
}

func (j *journal) list() []string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]string(nil), j.events...)
}

type response struct {
	mu     sync.Mutex
	status int
	body   string
}

func (r *response) get() (int, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status, r.body
}

func (r *response) set(status int, body string) {
	r.mu.Lock()
	r.status, r.body = status, body
	r.mu.Unlock()
}

type rig struct {
	svc     *Service
	api     *fakeAPI
	journal *journal
	resp    *response
}

type rigOptions struct {
	token      func() (string, error)
	uploadLogs func() (string, error)
	uploadCtx  func(ctx context.Context) (string, error)
	noUploader bool
	status     int
	body       string
	language   string
}

func newRig(t *testing.T, opts rigOptions) *rig {
	t.Helper()
	j := &journal{}
	resp := &response{status: opts.status, body: opts.body}
	if resp.status == 0 {
		resp.status, resp.body = http.StatusCreated, `{"ticket":`+ticketJSON+`,"message":`+messageJSON+`}`
	}
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		j.add(r.Method + " " + r.URL.Path)
		status, body := resp.get()
		reply(t, w, status, body)
	})
	token := opts.token
	if token == nil {
		token = staticToken("tok")
	}
	language := opts.language
	if language == "" {
		language = "ru"
	}
	deps := Deps{Language: func() string { return language }}
	if !opts.noUploader {
		upload := opts.uploadLogs
		if upload == nil {
			upload = func() (string, error) { return "BUNDLE01", nil }
		}
		deps.UploadLogs = func(ctx context.Context) (string, error) {
			j.add("upload")
			if opts.uploadCtx != nil {
				return opts.uploadCtx(ctx)
			}
			return upload()
		}
	}
	svc, err := NewService(api.server.URL, token, deps)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.info = func() ClientInfo { return testInfo }
	return &rig{svc: svc, api: api, journal: j, resp: resp}
}

func (r *rig) start(t *testing.T) {
	t.Helper()
	if err := r.svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	t.Cleanup(func() {
		if err := r.svc.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
}

func decodeBody(t *testing.T, req seen) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(req.body, &got); err != nil {
		t.Fatalf("request body %q: %v", req.body, err)
	}
	return got
}

func validInput() CreateInput {
	return CreateInput{Topic: TopicInstall, Subject: "  Won't install  ", Body: "\n details \n"}
}

func TestCreateFillsClientAndLanguage(t *testing.T) {
	for _, language := range []string{"ru", "en"} {
		t.Run(language, func(t *testing.T) {
			r := newRig(t, rigOptions{language: language})
			r.start(t)
			game := int64(1942)
			input := validInput()
			input.GameIgdbID = &game

			ticket, err := r.svc.Create(input)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			if ticket.Number != 1042 {
				t.Fatalf("ticket = %+v", ticket)
			}
			req := r.api.only()
			if req.method != http.MethodPost || req.path != "/v1/support/tickets" {
				t.Fatalf("request = %s %s", req.method, req.path)
			}
			got := decodeBody(t, req)
			if got["lang"] != language {
				t.Fatalf("lang = %v, want %q", got["lang"], language)
			}
			if got["subject"] != "Won't install" || got["body"] != "details" || got["topic"] != "install" || got["gameIgdbId"] != float64(1942) {
				t.Fatalf("body = %v", got)
			}
			client := as[map[string]any](t, got["client"])
			if client["appVersion"] != "9.9.9" || client["os"] != "windows" || client["arch"] != "amd64" {
				t.Fatalf("client = %v", got["client"])
			}
			if _, has := got["logBundleId"]; has {
				t.Fatalf("logBundleId sent without logs: %v", got)
			}
			if events := r.journal.list(); len(events) != 1 {
				t.Fatalf("events = %v, want no upload", events)
			}
		})
	}
}

func TestCreateWithoutUILanguageSendsNoLang(t *testing.T) {
	r := newRig(t, rigOptions{})
	r.svc.language = func() string { return "" }
	r.start(t)

	if _, err := r.svc.Create(validInput()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got := decodeBody(t, r.api.only())
	if _, has := got["lang"]; has {
		t.Fatalf("lang = %v, want the field left out", got["lang"])
	}
}

func TestReplyBodyHasOnlyTheFieldsTheAPIKnows(t *testing.T) {
	r := newRig(t, rigOptions{language: "en"})
	r.start(t)

	if _, err := r.svc.Reply(1042, "more", true); err != nil {
		t.Fatalf("Reply: %v", err)
	}
	for key := range decodeBody(t, r.api.only()) {
		if key != "body" && key != "logBundleId" {
			t.Fatalf("reply body carries %q, which the API refuses as an unknown field", key)
		}
	}
}

func TestCreateClientInfoComesFromTheBuild(t *testing.T) {
	r := newRig(t, rigOptions{})
	r.svc.info = appClientInfo
	r.start(t)

	if _, err := r.svc.Create(validInput()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	client := as[map[string]any](t, decodeBody(t, r.api.only())["client"])
	if client["appVersion"] == "" || client["os"] == "" || client["arch"] == "" {
		t.Fatalf("client = %v, want version, os and arch", client)
	}
}

func TestCreateUploadsLogsFirstAndAttachesTheirID(t *testing.T) {
	r := newRig(t, rigOptions{})
	r.start(t)
	input := validInput()
	input.AttachLogs = true

	if _, err := r.svc.Create(input); err != nil {
		t.Fatalf("Create: %v", err)
	}
	events := r.journal.list()
	if len(events) != 2 || events[0] != "upload" || events[1] != "POST /v1/support/tickets" {
		t.Fatalf("events = %v, want the upload before the ticket", events)
	}
	if got := decodeBody(t, r.api.only()); got["logBundleId"] != "BUNDLE01" {
		t.Fatalf("logBundleId = %v", got["logBundleId"])
	}
}

func TestReplyUploadsLogsFirstAndAttachesTheirID(t *testing.T) {
	r := newRig(t, rigOptions{})
	r.start(t)

	msg, err := r.svc.Reply(1042, " more info ", true)
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if msg.ID != 7 {
		t.Fatalf("message = %+v", msg)
	}
	events := r.journal.list()
	if len(events) != 2 || events[0] != "upload" || events[1] != "POST /v1/support/tickets/1042/messages" {
		t.Fatalf("events = %v", events)
	}
	got := decodeBody(t, r.api.only())
	if got["logBundleId"] != "BUNDLE01" || got["body"] != "more info" {
		t.Fatalf("body = %v", got)
	}
}

func TestReplyWithoutLogsDoesNotUpload(t *testing.T) {
	r := newRig(t, rigOptions{})
	r.start(t)

	if _, err := r.svc.Reply(1042, "hello", false); err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if events := r.journal.list(); len(events) != 1 {
		t.Fatalf("events = %v, want no upload", events)
	}
	if _, has := decodeBody(t, r.api.only())["logBundleId"]; has {
		t.Fatal("logBundleId sent without logs")
	}
}

func TestFailedUploadCreatesNothing(t *testing.T) {
	cases := []struct {
		name     string
		upload   func() (string, error)
		noUpload bool
		wantCode string
	}{
		{
			name:     "coded upload error keeps its code",
			upload:   func() (string, error) { return "", uierr.New(diagnostics.ErrCodeLogUploadRateLimited, "429") },
			wantCode: diagnostics.ErrCodeLogUploadRateLimited,
		},
		{
			name:     "uncoded upload error gets the failed code",
			upload:   func() (string, error) { return "", errors.New("log files are locked") },
			wantCode: diagnostics.ErrCodeLogUploadFailed,
		},
		{
			name:     "empty bundle id is a failed upload",
			upload:   func() (string, error) { return "  ", nil },
			wantCode: diagnostics.ErrCodeLogUploadFailed,
		},
		{
			name:     "no uploader in this session",
			noUpload: true,
			wantCode: "support.logs_unavailable",
		},
	}
	for _, tc := range cases {
		for _, op := range []string{"create", "reply"} {
			t.Run(tc.name+"/"+op, func(t *testing.T) {
				r := newRig(t, rigOptions{uploadLogs: tc.upload, noUploader: tc.noUpload})
				r.start(t)

				var err error
				if op == "create" {
					input := validInput()
					input.AttachLogs = true
					_, err = r.svc.Create(input)
				} else {
					_, err = r.svc.Reply(1042, "hello", true)
				}
				if err == nil {
					t.Fatal("the call succeeded without the logs it was asked to attach")
				}
				if code := uierr.Code(err); code != tc.wantCode {
					t.Fatalf("code = %q, want %q (err: %v)", code, tc.wantCode, err)
				}
				for _, event := range r.journal.list() {
					if event != "upload" {
						t.Fatalf("request %q went out after a failed upload", event)
					}
				}
			})
		}
	}
}

func TestAttachLogsSignedOutUploadsNothing(t *testing.T) {
	tokens := map[string]func() (string, error){
		"empty token":   staticToken(""),
		"no credential": func() (string, error) { return "", account.ErrNoCredential },
	}
	for name, token := range tokens {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, rigOptions{token: token})
			r.start(t)
			input := validInput()
			input.AttachLogs = true

			_, err := r.svc.Create(input)
			if apiErr := apiCode(t, err); apiErr.Code != account.CodeUnauthenticated {
				t.Fatalf("code = %q", apiErr.Code)
			}
			if events := r.journal.list(); len(events) != 0 {
				t.Fatalf("events = %v, want nothing", events)
			}
		})
	}
}

func TestAttachLogsTokenFailureIsNotSignedOut(t *testing.T) {
	broken := errors.New("keychain locked")
	r := newRig(t, rigOptions{token: func() (string, error) { return "", broken }})
	r.start(t)

	_, err := r.svc.Reply(1, "hello", true)
	if !errors.Is(err, broken) {
		t.Fatalf("error = %v, want the keychain failure", err)
	}
	if events := r.journal.list(); len(events) != 0 {
		t.Fatalf("events = %v, want nothing", events)
	}
}

func TestCreateRejectsBadInputBeforeAnythingIsSent(t *testing.T) {
	zero := int64(0)
	negative := int64(-3)
	cases := []struct {
		name      string
		mutate    func(in *CreateInput)
		wantCode  string
		wantField string
	}{
		{"unknown topic", func(in *CreateInput) { in.Topic = "billing" }, CodeTicketInvalidTopic, "topic"},
		{"empty topic", func(in *CreateInput) { in.Topic = "" }, CodeTicketInvalidTopic, "topic"},
		{"topic with different case", func(in *CreateInput) { in.Topic = "Install" }, CodeTicketInvalidTopic, "topic"},
		{"empty subject", func(in *CreateInput) { in.Subject = "" }, CodeTicketInvalidSubject, "subject"},
		{"blank subject", func(in *CreateInput) { in.Subject = " \t " }, CodeTicketInvalidSubject, "subject"},
		{"subject with a line break", func(in *CreateInput) { in.Subject = "one\ntwo" }, CodeTicketInvalidSubject, "subject"},
		{"subject with a carriage return", func(in *CreateInput) { in.Subject = "one\rtwo" }, CodeTicketInvalidSubject, "subject"},
		{"subject over the limit", func(in *CreateInput) { in.Subject = strings.Repeat("a", MaxSubjectRunes+1) }, CodeTicketInvalidSubject, "subject"},
		{"empty body", func(in *CreateInput) { in.Body = "" }, CodeTicketInvalidBody, "body"},
		{"blank body", func(in *CreateInput) { in.Body = "  \n " }, CodeTicketInvalidBody, "body"},
		{"body over the limit", func(in *CreateInput) { in.Body = strings.Repeat("a", MaxBodyRunes+1) }, CodeTicketInvalidBody, "body"},
		{"zero game", func(in *CreateInput) { in.GameIgdbID = &zero }, CodeTicketInvalidGame, "gameIgdbId"},
		{"negative game", func(in *CreateInput) { in.GameIgdbID = &negative }, CodeTicketInvalidGame, "gameIgdbId"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, rigOptions{})
			r.start(t)
			input := validInput()
			input.AttachLogs = true
			tc.mutate(&input)

			_, err := r.svc.Create(input)
			apiErr := apiCode(t, err)
			if apiErr.Code != tc.wantCode || apiErr.Field != tc.wantField {
				t.Fatalf("error = {%q %q}, want {%q %q}", apiErr.Code, apiErr.Field, tc.wantCode, tc.wantField)
			}
			if events := r.journal.list(); len(events) != 0 {
				t.Fatalf("events = %v: logs were uploaded or a request sent for input the server would refuse", events)
			}
		})
	}
}

func TestLengthLimitsCountRunes(t *testing.T) {
	r := newRig(t, rigOptions{})
	r.start(t)
	input := validInput()
	input.Subject = strings.Repeat("я", MaxSubjectRunes)
	input.Body = strings.Repeat("ы", MaxBodyRunes)

	if _, err := r.svc.Create(input); err != nil {
		t.Fatalf("Create at the limit in multi-byte text: %v", err)
	}
}

func TestReplyRejectsBadInputBeforeAnythingIsSent(t *testing.T) {
	cases := []struct {
		name     string
		number   int64
		body     string
		wantCode string
	}{
		{"empty body", 1, "", CodeTicketInvalidBody},
		{"blank body", 1, "  ", CodeTicketInvalidBody},
		{"body over the limit", 1, strings.Repeat("a", MaxBodyRunes+1), CodeTicketInvalidBody},
		{"zero number", 0, "hello", CodeTicketNotFound},
		{"negative number", -4, "hello", CodeTicketNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, rigOptions{})
			r.start(t)

			_, err := r.svc.Reply(tc.number, tc.body, true)
			if apiErr := apiCode(t, err); apiErr.Code != tc.wantCode {
				t.Fatalf("code = %q, want %q", apiErr.Code, tc.wantCode)
			}
			if events := r.journal.list(); len(events) != 0 {
				t.Fatalf("events = %v", events)
			}
		})
	}
}

func TestServerRefusalReachesTheCaller(t *testing.T) {
	r := newRig(t, rigOptions{status: http.StatusConflict, body: `{"error":{"code":"ticket_limit"}}`})
	r.start(t)
	input := validInput()
	input.AttachLogs = true

	_, err := r.svc.Create(input)
	if apiErr := apiCode(t, err); apiErr.Code != CodeTicketLimit {
		t.Fatalf("code = %q", apiErr.Code)
	}
	if _, err := r.svc.Reply(1, "hello", false); apiCode(t, err).Code != CodeTicketLimit {
		t.Fatalf("reply error = %v", err)
	}
}

func TestReadCallsReachTheAPI(t *testing.T) {
	r := newRig(t, rigOptions{status: http.StatusOK, body: `{"tickets":[],"next":"","ticket":` + ticketJSON + `,"messages":[],"count":4}`})
	r.start(t)

	if page, err := r.svc.List("c1"); err != nil || page.Tickets == nil {
		t.Fatalf("List = %+v, %v", page, err)
	}
	if detail, err := r.svc.Get(1042); err != nil || detail.Ticket.Number != 1042 {
		t.Fatalf("Get = %+v, %v", detail, err)
	}
	if ticket, err := r.svc.Resolve(1042); err != nil || ticket.Number != 1042 {
		t.Fatalf("Resolve = %+v, %v", ticket, err)
	}
	if n, err := r.svc.Unread(); err != nil || n != 4 {
		t.Fatalf("Unread = %d, %v", n, err)
	}
	want := []string{
		"GET /v1/support/tickets",
		"GET /v1/support/tickets/1042",
		"POST /v1/support/tickets/1042/resolve",
		"GET /v1/support/unread",
	}
	got := r.journal.list()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("requests = %v, want %v", got, want)
	}
}

func TestUnreadSignedOutIsUnauthenticatedWithoutARequest(t *testing.T) {
	r := newRig(t, rigOptions{token: staticToken("")})
	r.start(t)

	_, err := r.svc.Unread()
	if apiErr := apiCode(t, err); apiErr.Code != account.CodeUnauthenticated {
		t.Fatalf("code = %q", apiErr.Code)
	}
	if n := len(r.api.requests()); n != 0 {
		t.Fatalf("%d requests went out", n)
	}
}

func TestCallsBeforeStartupAndAfterShutdown(t *testing.T) {
	calls := map[string]func(s *Service) error{
		"list":    func(s *Service) error { _, err := s.List(""); return err },
		"get":     func(s *Service) error { _, err := s.Get(1); return err },
		"resolve": func(s *Service) error { _, err := s.Resolve(1); return err },
		"unread":  func(s *Service) error { _, err := s.Unread(); return err },
		"create":  func(s *Service) error { _, err := s.Create(validInput()); return err },
		"reply":   func(s *Service) error { _, err := s.Reply(1, "hi", false); return err },
	}
	for name, call := range calls {
		t.Run(name+"/before startup", func(t *testing.T) {
			r := newRig(t, rigOptions{})
			if err := call(r.svc); !errors.Is(err, errNotStarted) {
				t.Fatalf("error = %v, want errNotStarted", err)
			}
			if events := r.journal.list(); len(events) != 0 {
				t.Fatalf("events = %v", events)
			}
		})
		t.Run(name+"/after shutdown", func(t *testing.T) {
			r := newRig(t, rigOptions{})
			if err := r.svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
				t.Fatalf("startup: %v", err)
			}
			if err := r.svc.ServiceShutdown(); err != nil {
				t.Fatalf("shutdown: %v", err)
			}
			err := call(r.svc)
			if err == nil {
				t.Fatal("call succeeded after shutdown")
			}
			if n := len(r.api.requests()); n != 0 {
				t.Fatalf("%d requests went out after shutdown", n)
			}
		})
	}
}

func TestCreateBeforeStartupDoesNotUploadLogs(t *testing.T) {
	r := newRig(t, rigOptions{})
	input := validInput()
	input.AttachLogs = true

	if _, err := r.svc.Create(input); !errors.Is(err, errNotStarted) {
		t.Fatalf("error = %v, want errNotStarted", err)
	}
	if events := r.journal.list(); len(events) != 0 {
		t.Fatalf("events = %v", events)
	}
}

func TestShutdownCancelsAPendingCall(t *testing.T) {
	entered := make(chan struct{})
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
	})
	svc, err := NewService(api.server.URL, staticToken("tok"), Deps{Language: func() string { return "ru" }})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := svc.Unread()
		done <- err
	}()

	<-entered
	if err := svc.ServiceShutdown(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestLimits(t *testing.T) {
	r := newRig(t, rigOptions{})
	if got := r.svc.Limits(); got != (Limits{MaxSubjectRunes: 120, MaxBodyRunes: 5000}) {
		t.Fatalf("limits = %+v", got)
	}
}

func TestNewServiceRejectsBadSetup(t *testing.T) {
	language := func() string { return "ru" }
	cases := map[string]struct {
		base  string
		token func() (string, error)
		deps  Deps
	}{
		"no language source": {"https://api.example.com", staticToken("t"), Deps{}},
		"empty base":         {"", staticToken("t"), Deps{Language: language}},
		"plain http":         {"http://api.example.com", staticToken("t"), Deps{Language: language}},
		"no token source":    {"https://api.example.com", nil, Deps{Language: language}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewService(tc.base, tc.token, tc.deps); err == nil {
				t.Fatal("NewService accepted it")
			}
		})
	}
}
