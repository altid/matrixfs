package ircfs

import (
	"context"

	"github.com/altid/matrixfs/internal/commands"
	"github.com/altid/matrixfs/internal/session"
	"github.com/altid/libs/config"
	"github.com/altid/libs/mdns"
	"github.com/altid/libs/service"
	"github.com/altid/libs/service/listener"
	"github.com/altid/libs/store"
)

type Matrixfs struct {
	run	func() error
	session	*session.Session
	name	string
	debug	bool
	mdns	*mdns.Entry
	ctx	context.Context
}

func CreateConfig(srv string, debug bool) error {
	d := &session.Defaults{}
	return config.Create(d, srv, "", debug)
}

func Register(ldir bool, addr, srv string, debug bool) (*Matrixfs, error) {
	defaults := &session.Defaults{
		Address:	"matrix.chat",
		Port:	443,
		SSL:	"simple",
		Auth:	"password",
		User:	"guest",
		Name:	"guest",
		Logdir:	"",
		TLSCert:	"",
		TLSKey:	"",
	}

	if e := config.Marshal(defaults, srv, "", debug); e != nil {
		return nil, e
	}

	l, err := tolisten(defaults, addr, debug)
	if err != nil {
		return nil, err
	}

	s := tostore(defaults, ldir, debug)
	session := &session.Session{
		Defaults: defaults,
		Verbose: debug,
	}

	session.Parse()
	ctx := context.Background()

	m := &Matrixfs{
		session:	session,
		ctx:		ctx,
		name:		srv,
		debug:		debug,
	}

	c := service.New(srv, addr, debug)
	c.WithListener(l)
	c.WithStore(s)
	c.WithContext(ctx)
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
	entry := &mdns.Entry {
		Addr: matrix.session.Defaults.Address,
		Name: matrix.name,
		Txt: nil,
		Port: matrix.session.Defaults.Port,
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

func tolisten(d *session.Defaults, addr string, debug bool) (listener.Listener, error) {
	//if ssh {
	// 	return listener.NewListenSsh()
	//}

	if d.TLSKey == "none" && d.TLSCert == "none" {
		return listener.NewListen9p(addr, "" , "", debug)
	}

	return listener.NewListen9p(addr, d.TLSCert, d.TLSKey, debug)
}

func tostore(d *session.Defaults, ldir, debug bool) store.Filer {
	if ldir {
		return store.NewLogStore(d.Logdir.String(), debug)
	}

	return store.NewRamStore(debug)
}
