package ircfs

import (
	"context"
	"fmt"
	"net/url"

	//"strings"

	"github.com/altid/libs/config"
	"github.com/altid/libs/mdns"
	"github.com/altid/libs/service"
	"github.com/altid/libs/service/listener"
	"github.com/altid/libs/store"
	"github.com/altid/matrixfs/internal/commands"
	"github.com/altid/matrixfs/internal/session"
)

type Matrixfs struct {
	run	func() error
	session	*session.Session
	name	string
	debug	bool
	mdns	*mdns.Entry
	ctx	context.Context
}

var defaults *session.Defaults = &session.Defaults{
	Address:	"https://matrix.chat",
	Port:		443,
	SSL:		"simple",
	Auth:		"password",
	User:		"guest",
	Logdir:		"",
	TLSCert:	"",
	TLSKey:		"",
}

func CreateConfig(srv string, debug bool) error {
	return config.Create(defaults, srv, "", debug)
}

func Register(ldir bool, addr string, port int, srv string, debug bool) (*Matrixfs, error) {
	var err error
	if e := config.Marshal(defaults, srv, "", debug); e != nil {
		return nil, e
	}
	if defaults.Address, err = toaddr(defaults); err != nil {
		return nil, err
	}
	l, err := tolisten(defaults, addr, port, debug)
	if err != nil {
		return nil, err
	}
	s := tostore(defaults, ldir, debug)
	session := session.NewSession(defaults, debug)
	session.Parse()
	m := &Matrixfs{
		session:	session,
		name:		srv,
		debug:		debug,
	}
	c := service.New(srv, addr, debug)
	c.WithListener(l)
	c.WithStore(s)
	c.WithContext(session.Ctx)
	c.WithCallbacks(session)
	c.WithRunner(session)
	c.SetCommands(commands.Commands)
	m.run = c.Listen
	return m, nil
}

func (matrix *Matrixfs) Run() error {
	return matrix.run()
}

func (matrix *Matrixfs) Broadcast() error {
	url := fmt.Sprintf("%s:%d", matrix.session.Defaults.Address, matrix.session.Defaults.Port)
	entry, err := mdns.ParseURL(url, matrix.name)
	if err != nil {
		return err
	}
	if e := mdns.Register(entry); e != nil {
		return e
	}

	matrix.mdns = entry
	return nil
}

func (matrix *Matrixfs) Cleanup() {
	if matrix.mdns != nil {
		matrix.mdns.Cleanup()
	}
	matrix.session.Quit()
}

func (matrix *Matrixfs) Session() *session.Session {
	return matrix.session
}

func tolisten(d *session.Defaults, addr string, port int, debug bool) (listener.Listener, error) {
	//if ssh {
	// 	return listener.NewListenSsh()
	//}

	dial := fmt.Sprintf("%s:%d", addr, port)
	if d.TLSKey == "none" && d.TLSCert == "none" {
		return listener.NewListen9p(dial, "" , "", debug)
	}

	return listener.NewListen9p(dial, d.TLSCert, d.TLSKey, debug)
}

func tostore(d *session.Defaults, ldir, debug bool) store.Filer {
	if ldir {
		return store.NewLogstore(d.Logdir.String(), debug)
	}

	return store.NewRamstore(debug)
}

func toaddr(d *session.Defaults) (string, error) {
	// Sanitize our URL
	dial, err := url.Parse(d.Address)
	if err != nil {
		return "", err
	}
	if(!dial.IsAbs()) {
		dial.Scheme = "https"
		// Because... adding a scheme doesn't set the host correctly
		dial, _ = url.Parse(dial.String())
	}
	return dial.String(), nil
}
