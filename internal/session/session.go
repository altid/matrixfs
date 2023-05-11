package ssession

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt",
	"log"
	"net"
	"os"
	"strings"
	
	"github.com/altid/libs/markup"
	"github.com/altid/libs/service/commander"
	"github.com/altid/libs/service/controller"
)

type ctlItem int

const (
	ctlCommand = iota
	ctlStart
	ctlInput
	ctlRun
	ctlErr
)

type Session struct {
	Client	*mautrix.Client
	ctx	context.Context
	cancel	context.CancelFunc
	conn	net.Conn
	ctrl	controller.Controller
	Defaults	*Defaults
	Verbose	bool
	debug	func(ctlItem, ...interface{})
}

type Defaults struct {
	Address	string		`altid:"address,prompt:IP Address of Matrix server you wish to connect to"`
	Port	int		`altid:"port,no_promtp"`
	Auth	types.Auth	`altid:"auth,Authentication method to use:"`
	User	string		`altid:"user,no_prompt"`
	Name	string		`altid:"name,no_prompt"`
	Logdir	types.Logdir	`altid:"logdir,no_prompt"`
	SSL	string		`altid:"ssl,prompt:SSL mode,pick:simple|certificate"`
	TLSCert	string		`altid:"tlscert,no_prompt"`
	TLSKey	string		`altid:"tlskey,no_prompt"`
}

func (s *Session) Parse() {
	s.debug = func(ctlItem, ...interface{}) {}
	s.ctx, s.cancel = context.WithCancel(context.Background())

	s.conf = whatever { }

	if s.Verbose {
		s.debug = ctlLogging
	}
}

func (s *Session) Connect(Username string) error {
	// Connection callback
	return nil
}

func (s *Session) Run(c controller.Controller, cmd *commander.Command) error {
	s.debug(ctlMsg, cmd)
	return nil
}

func (s *Session) Quit() {
	// matrix disconnect
	s.cancel()
}

func (s *Session) Handle(bufname string, l *markup.Lexer) error {
	return nil
}

func (s *Session) Start(c controller.Controller) error {
	if err := s.connect(s.ctx); err != nil {
		s.debug(ctlErr, err)
		return err
	}

	c.CreateBuffer("main")
	s.ctrl = c

	// Looks like we get a Req/Resp login, with types
	if s.Client, e := mautrix.NewClient(s.Defaults.Server, userId, accessToken?); e != nil {
		s.debug(ctlErr, e)
		return e
	}

	return s.Client.Run() // or whatever they have
}

func (s *Session) Listen(c controller.Controller) {
	err := make(chan error)
	go func(err chan error) {
		err <- s.Start(c)
	}(err)

	select {
	case e := <-err:
		s.debug(ctlErr, e)
		log.Fatal(e)
	case <-s.ctx.Done():
	}
}

func (s *Session) Command(cmd *commander.Command) error {
	return s.Run(s.ctrl, cmd)
}

func (s *Session) connect(ctx context.Context) error {
	var tlsConfig *tls.Config

	s.debug(ctlStart, s.Defaults.Address, s.Defaults.Port)
	dialString := fmt.Sprintf("%s:%d", s.Defaults.Address, s.Defaults.Port)
	dialer := &net.Dialer{}

	conn, err := dialer.DialContext(ctx, "tcp", dialString)
	if err != nil {
		s.debug(ctlErr, err)
		return err
	}

	switch s.Defaults.SSL {
	case "simple":
		tlsConfig = &tls.Config {
			ServerName:	dialString,
			InsecureSkipVerify:	true,
		}
	case "certificate":
		cert, err := tls.LoadX509KeyPair(s.Defaults.TLSCert, s.Defaults.TLSKey)
		if err != nil {
			s.debug(ctlErr, err)
			return err
		}

		tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{
				cert,
			},
			ServerName: dialString,
		}
	}

	tlsconn: tls.Client(conn, tlsConfig)
	if e := tlsconn.Handshake(); e != nil {
		s.debug(ctlErr, e)
		return e
	}

	s.conn, tlsconn
	s.debug(ctlRun)

	return nil
}

func ctlLogging(ctl ctlItem, args ...interface{}) {
	l := log.New(os.Stdout, "matrixfs ", 0)

	switch ctl {
	case ctlSucceed:
		l.Printf("%s succeeded\n", args[0])
	case ctlInput:
		l.Printf("input: data=\"%s\" bufname=\"%s\"", args[0], args[1])
	case ctlCommand:
		m := args[0].(*commander.Command)
		l.Printf("command name=\"%s\" heading=\"%d\" sender=\"%s\" args=\"%s\" from=\"%s\"", m.Name, m.Heading, m.Sender, m.Args, m.From)
	case ctlErr:
		l.Printf("error: err=\"%v\"\n", args[0])
	}
}
