package gsarangodb

import (
	"bitbucket.org/starJammer/arango"
	"fmt"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"net/http"
	"os"
	"testing"
)

var (
	db *arango.Database
	c  *arango.Collection
)

type dumbResponseWriter struct {
	header http.Header
}

func (r dumbResponseWriter) Header() http.Header {
	if r.header == nil {
		r.header = make(http.Header)
	}
	return r.header
}

func (r dumbResponseWriter) Write([]byte) (int, error) {
	return 10, nil
}

func (r dumbResponseWriter) WriteHeader(int) {

}

func TestMain(m *testing.M) {
	var err error
	db, _ := arango.ConnDbUserPassword("http://localhost:8529", "_system", "root", "")
	db.DropDatabase("test")
	_ = db.CreateDatabase(
		"test",
		nil,
		[]arango.User{
			arango.User{Username: "root", Passwd: "", Active: true},
		})
	db, _ = db.UseDatabase("test")
	c, err = db.CreateDocumentCollection("sessions")

	if err != nil {
		fmt.Printf("Aborting test...\n%s\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestArangoDbNoOptions(t *testing.T) {
	_, err := NewArangoDbStore(nil)
	if err == nil {
		t.Error("Expected an error with nil options but didn't get one.")
	}
}

// Test for GH-8 for CookieStore
func TestArangoDbStore(t *testing.T) {
	store, err := NewArangoDbStore(&ArangoDbOptions{Collection: c}, []byte("codec-key"))

	if err != nil {
		t.Fatal(err)
	}

	originalPath := "/"
	store.SessionOptions.Path = originalPath

	req, err := http.NewRequest("GET", "http://www.example.com", nil)
	if err != nil {
		t.Fatal("failed to create request", err)
	}

	session, err := store.New(req, "hello")
	if err != nil {
		t.Fatal("failed to create session", err)
	}

	store.SessionOptions.Path = "/foo"
	if session.Options.Path != originalPath {
		t.Fatalf("bad session path: got %q, want %q", session.Options.Path, originalPath)
	}
	session.Values["test"] = "test"

	res := dumbResponseWriter{}
	err = store.Save(req, res, session)

	if err != nil {
		t.Fatal(err)
	}

	if session.ID == "" {
		t.Fatal("Expected the session to have an id set but it was blank still.")
	}

	//Add the cookie to the request so we can attempt to retrieve the new session
	encoded, err := securecookie.EncodeMulti(session.Name(), session.ID, store.Codecs...)
	req.AddCookie(sessions.NewCookie(session.Name(), encoded, session.Options))

	session2, err := store.New(req, "hello")

	if err != nil {
		t.Fatal(err)
	}

	if v, ok := session2.Values["test"].(string); !ok || v != "test" {
		t.Fatalf("Could not retrieve values from arangodb: %+v\n", session2.Values)
	}

}
