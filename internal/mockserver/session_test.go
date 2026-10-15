package mockserver_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/daroco/terraform-provider-satisfactory/internal/client"
	"github.com/daroco/terraform-provider-satisfactory/internal/mockserver"
)

func sessionServer(t *testing.T) (*mockserver.Server, *client.Client) {
	t.Helper()
	s := mockserver.New("")
	s.Seed(mockserver.SampleHandBuiltWorld())
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, client.New(srv.URL, "")
}

func TestSaveSessionDefaultsTheName(t *testing.T) {
	s, c := sessionServer(t)
	res, err := c.SaveSession(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Saved != "mock_terraform" {
		t.Errorf("saved = %q, want mock_terraform", res.Saved)
	}
	res, err = c.SaveSession(context.Background(), "before-apply")
	if err != nil {
		t.Fatal(err)
	}
	if res.Saved != "before-apply" {
		t.Errorf("saved = %q", res.Saved)
	}
	if got := s.SavedNames(); len(got) != 2 || got[1] != "before-apply" {
		t.Errorf("saved names = %v", got)
	}
	if s.QuitRequested() {
		t.Error("a save must not quit")
	}
}

func TestQuitSessionSavesUnlessToldNotTo(t *testing.T) {
	s, c := sessionServer(t)
	res, err := c.QuitSession(context.Background(), true, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Saved != "mock_terraform" || !s.QuitRequested() {
		t.Errorf("quit with save: %+v, quit=%v", res, s.QuitRequested())
	}

	s2, c2 := sessionServer(t)
	res, err = c2.QuitSession(context.Background(), false, "ignored")
	if err != nil {
		t.Fatal(err)
	}
	if res.Saved != "" || len(s2.SavedNames()) != 0 || !s2.QuitRequested() {
		t.Errorf("quit without save: %+v, saves=%v, quit=%v", res, s2.SavedNames(), s2.QuitRequested())
	}
}

func TestGiveItems(t *testing.T) {
	_, c := sessionServer(t)
	res, err := c.GiveItems(context.Background(), "pioneer", "Desc_Coal_C", 100)
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 100 || res.Requested != 100 || res.Player != "pioneer" || res.Item != "Desc_Coal_C" {
		t.Errorf("grant = %+v", res)
	}

	// Count defaults to one.
	res, err = c.GiveItems(context.Background(), "pioneer", "Desc_Coal_C", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 1 {
		t.Errorf("default count: %+v", res)
	}

	var apiErr *client.APIError
	if _, err := c.GiveItems(context.Background(), "nobody", "Desc_Coal_C", 1); !client.IsNotFound(err) {
		t.Errorf("unknown player: err = %v, want not found", err)
	}
	if _, err := c.GiveItems(context.Background(), "pioneer", "Build_ConstructorMk1_C", 1); !errors.As(err, &apiErr) || apiErr.Status != 422 {
		t.Errorf("non-item class: err = %v, want 422", err)
	}
	if _, err := c.GiveItems(context.Background(), "pioneer", "Desc_Coal_C", -5); !errors.As(err, &apiErr) || apiErr.Status != 422 {
		t.Errorf("negative count: err = %v, want 422", err)
	}
}
