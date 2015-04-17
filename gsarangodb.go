package gsarangodb

import (
	"github.com/starJammer/arango"
	"errors"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"net/http"
)

var (
	NoOptionsSetErr = errors.New("You must provide a set of valid options")
)

type ArangoDbStore struct {
	Codecs         []securecookie.Codec
	collection     *arango.Collection
	SessionOptions *sessions.Options
}

//ArangoDbOptions holds options for the ArangoDbStore, such as connection info. It also lets you provide
type ArangoDbOptions struct {

	//CollectionName is if you have created your own session collection. Otherwise, the "sessions" name will be used
	CollectionName string

	//Use these options if you don't already have a connection available and want to create a new one
	Host         string //http://localhost:8529 will be used if not specified
	DatabaseName string //_system will be used if not specified
	User         string //root will be used if not specified
	Password     string //root's blank password will be used if not specified

	//Set this if you already have an arango connection and want to use that instead.
	//You should still set the CollectionName unless you want to use the default "sessions" collection
	Database *arango.Database

	//Setting this will cause all other options to be ignored. This collection will be used write sessions to.
	Collection *arango.Collection

	//SessionOptions specifies options for the sessions that this store will create. See github.com/gorilla/sessions
	SessionOptions *sessions.Options
}

func NewArangoDbStore(opts *ArangoDbOptions, keyPairs ...[]byte) (*ArangoDbStore, error) {
	store := &ArangoDbStore{}

	store.Codecs = securecookie.CodecsFromPairs(keyPairs...)

	if opts == nil {
		return nil, NoOptionsSetErr
	}

	if opts.SessionOptions == nil {
		store.SessionOptions = &sessions.Options{
			Path:   "/",
			MaxAge: 86400 * 7,
		}
	} else {
		store.SessionOptions = opts.SessionOptions
	}

	var err error

	if opts.Collection != nil {
		store.collection = opts.Collection
		return store, nil
	}

	if opts.Database != nil {
		store.collection, err = opts.Database.Collection(opts.CollectionName)
		if err != nil {
			return nil, err
		}
		return store, nil
	}

	if opts.Host == "" {
		opts.Host = "http://localhost:8529"
	}

	if opts.DatabaseName == "" {
		opts.DatabaseName = "_system"
	}

	if opts.User == "" {
		opts.User = "root"
		opts.Password = ""
	}

	//Attempt to connect with the connection details provided
	db, err := arango.ConnDbUserPassword(
		opts.Host,
		opts.DatabaseName,
		opts.User,
		opts.Password,
	)

	if err != nil {
		return nil, err
	}

	store.collection, err = db.Collection(opts.CollectionName)

	if err != nil {
		return nil, err
	}

	return store, nil
}

func (a *ArangoDbStore) Get(r *http.Request, name string) (*sessions.Session, error) {
	return sessions.GetRegistry(r).Get(a, name)
}

func (a *ArangoDbStore) New(r *http.Request, name string) (*sessions.Session, error) {
	session := sessions.NewSession(a, name)
	opts := *a.SessionOptions
	session.Options = &opts
	session.IsNew = true
	var err error
	if c, errCookie := r.Cookie(name); errCookie == nil {
		err = securecookie.DecodeMulti(name, c.Value, &session.ID, a.Codecs...)

		if err == nil {
			err = a.load(session)
			if err == nil {
				session.IsNew = false
			}
		}
	}
	return session, err
}

func (a *ArangoDbStore) Save(r *http.Request, w http.ResponseWriter, s *sessions.Session) error {

	if err := a.save(s); err != nil {
		return err
	}

	encoded, err := securecookie.EncodeMulti(s.Name(), s.ID, a.Codecs...)
	if err != nil {
		return err
	}
	http.SetCookie(w, sessions.NewCookie(s.Name(), encoded, s.Options))
	return nil
}

type sessionData struct {
	arango.DocumentImplementation
	SessionData string `json:"session-data"`
}

func (a *ArangoDbStore) load(s *sessions.Session) error {
	//ArangoKey of the document we'll be fetching
	id := s.ID
	data := &sessionData{}

	err := a.collection.Document(id, data)
	if err != nil {
		return err
	}

	if err = securecookie.DecodeMulti(s.Name(), data.SessionData, &s.Values, a.Codecs...); err != nil {
		return err
	}

	return nil
}

func (a *ArangoDbStore) save(s *sessions.Session) error {

	encoded, err := securecookie.EncodeMulti(s.Name(), s.Values, a.Codecs...)

	if err != nil {
		return nil
	}

	data := &sessionData{}
	if s.ID != "" {
		data.SetKey(s.ID)
	}
	data.SessionData = encoded
	err = a.collection.Save(data)

	if err != nil {
		return err
	}

	if s.ID == "" {
		s.ID = data.Key()
	}

	return nil
}
