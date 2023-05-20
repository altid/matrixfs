package session

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/altid/libs/config/types"
	"github.com/altid/libs/markup"
	"github.com/altid/libs/service/commander"
	"github.com/altid/libs/service/controller"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type ctlItem int

const (
	ctlCommand = iota
	ctlMsg
	ctlSucceed
	ctlStart
	ctlInput
	ctlLogin
	ctlLoginReq
	ctlInfo
	ctlMember
	ctlRun
	ctlErr
)

type Session struct {
	Defaults	*Defaults
	Verbose		bool
	Ctx			context.Context
	cancel		context.CancelFunc
	client		*mautrix.Client
	ctrl		controller.Controller
	rooms		map[string]string
	login		*mautrix.RespLogin
	debug		func(ctlItem, ...interface{})
}

type Defaults struct {
	Address	string			`altid:"address,prompt:IP Address of Matrix server you wish to connect to"`
	Auth	types.Auth		`altid:"auth,prompt:Authentication method to use"`
	User	string			`altid:"user,prompt:Matrix username"`
	Logdir	types.Logdir	`altid:"logdir,no_prompt"`
	Port	int				`altid:"port,no_prompt"`
	SSL		string			`altid:"ssl,prompt:SSL mode,pick:simple|certificate"`
	TLSCert	string			`altid:"tlscert,no_prompt"`
	TLSKey	string			`altid:"tlskey,no_prompt"`
}

// Sessions can be closed by the parent, or in our Quit() handler
func NewSession(defaults *Defaults, verbose bool) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	return &Session{
		Ctx:		ctx,
		cancel:		cancel,
		Defaults:	defaults,
		Verbose:	verbose,
		rooms: 		make(map[string]string),
	}
}

func (s *Session) Parse() {
	var err error
	s.debug = func(ctlItem, ...any) {}
	user := id.NewUserID(s.Defaults.Address, s.Defaults.User)
	if s.client, err = mautrix.NewClient(s.Defaults.Address, user, ""); err != nil {
		s.debug(ctlErr, err)
		os.Exit(1)
	}

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
	s.client.Logout()
	s.client.StopSync()
	s.cancel()
}

func (s *Session) Handle(bufname string, l *markup.Lexer) error {
	// TODO
	return nil
}

func (s *Session) Start(c controller.Controller) error {
	var e error
	s.ctrl = c
	s.debug(ctlSucceed, "connection")
	if e := s.connect(); e != nil {
		return e
	}
	
	// If we're a guest user, create a guest session
	if s.Defaults.User == "guest" {
		var resp *mautrix.RespRegister
		s.debug(ctlStart, "registration")
		reg := &mautrix.ReqRegister{
			Username: s.Defaults.User,
			Password: string(s.Defaults.Auth),
			DeviceID: "altid/matrixfs - guest",
		}
		if resp, _, e = s.client.RegisterGuest(reg); e != nil {
			return e
		}
		s.login.DeviceID = resp.DeviceID
		//s.login.HomeServer = resp.HomeServer
		s.login.UserID = resp.UserID
		s.login.AccessToken = resp.AccessToken
	} else {
		req := &mautrix.ReqLogin{
			Type:	 	mautrix.AuthTypePassword,
			Password:	string(s.Defaults.Auth),
			Identifier:	 mautrix.UserIdentifier{
				Type:	mautrix.IdentifierTypeUser,
				User: s.Defaults.User},
			StoreCredentials: true,
			DeviceID:	"altid/matrixfs",
		}
		s.debug(ctlLoginReq, req)
		if s.login, e = s.client.Login(req); e != nil {
			return e
		}
	}
	
	s.debug(ctlLogin, s.login)
	s.client.FullSyncRequest(mautrix.ReqSync{
		Context: s.Ctx,
		
	})
	// Sync handlers for the message types we need
	sync := s.client.Syncer.(*mautrix.DefaultSyncer)
	sync.OnEventType(event.EventMessage, s.message)
	sync.OnEventType(event.StateRoomName, s.name)
	sync.OnEventType(event.StateTopic, s.title)
	sync.OnEventType(event.StateRoomAvatar, s.avatar)
	sync.OnEventType(event.StateCreate, s.create)
	sync.OnEventType(event.StateMember, s.member)
	sync.OnEventType(event.EventRedaction, s.redaction)

	// Sync will call our event callbacks for us and keep up to date with the server
	s.debug(ctlSucceed, "Add sync events")
	ec := make(chan error)
	go func(ec chan error) {
		for {
			ec <- s.client.Sync()
		}
	}(ec)
	for {
		select {
		// Ensure we are checking our main context as well
		case <-s.Ctx.Done():
			return nil
		case e = <-ec:
			// TODO: Handle err appropriately
			return e
		}
	}
}

func (s *Session) Listen(c controller.Controller) {
	err := make(chan error)
	go func(err chan error) {
		err <- s.Start(c)
	}(err)
	select {
	case e := <-err:
		s.debug(ctlErr, e)
		s.cancel()
	case <-s.Ctx.Done():
	}
}

func (s *Session) Command(cmd *commander.Command) error {
	return s.Run(s.ctrl, cmd)
}

func (s *Session) title(src mautrix.EventSource, ev *event.Event) {
	title, err := s.ctrl.TitleWriter(s.rooms[ev.RoomID.String()])
	if err != nil {
		s.debug(ctlErr, err)
		return
	}
	fmt.Fprint(title, ev.Content.AsTopic().Topic)
}

func (s *Session) create(src mautrix.EventSource, ev *event.Event) {
	// We don't have the room name available here
	// Set to unknown, and try to find out before we log any real data
	if room, ok := s.rooms[ev.RoomID.String()]; ok {
		if room == "unknown" {
			log.Fatal("Multiple create events, unexpected")
		}
	}
	s.rooms[ev.RoomID.String()] = "unknown"
	//if e := s.ctrl.CreateBuffer(ev.RoomID); e != nil {
	//	s.debug(ctlErr, e)
	//}
	if e := s.client.MarkRead(ev.RoomID, ev.ID); e != nil {
		s.debug(ctlErr, e)
	}
}

func (s *Session) name(src mautrix.EventSource, ev *event.Event) {
	name := ev.Content.AsRoomName().Name
	s.rooms[ev.RoomID.String()] = name
	s.ctrl.CreateBuffer(name)
	if e := s.client.MarkRead(ev.RoomID, ev.ID); e != nil {
		s.debug(ctlErr, e)
	}
}

func (s *Session) avatar(src mautrix.EventSource, ev *event.Event) {
	// TODO: Build our title with an image instead
	s.client.MarkRead(ev.RoomID, ev.ID)
}

func (s *Session) redaction(src mautrix.EventSource, ev *event.Event) {
	if(ev.Sender == s.login.UserID) {
		return
	}
	/*
	from, ok := ev.PrevContent["ID"].(string)
	if !ok {
		return
	}
	// We don't log event IDs as we want to be plain-text sensible
	// TODO: We only try the last 50 messages, we could chunk further
	msg, err := s.client.Messages(ev.RoomID, from, ev.Redacts, 'f', 50)
	if err != nil {
		s.debug(ctlErr, err)
		return
	}
	for _, event := range msg.Chunk {
		if event.ID == ev.Redacts {
			if room, ok := s.rooms[event.RoomID]; ok {
				mw, err := s.ctrl.MainWriter(room)
				if err != nil {
					s.debug(ctlErr, err)
					break
				}
				fmt.Fprintf(mw, "s/%s/[redacted]/", event.Content["message"])
			}
			break
		}
	}
	*/
	if e := s.client.MarkRead(ev.RoomID, ev.ID); e != nil {
		s.debug(ctlErr, e)
	}
}

func (s *Session) member(src mautrix.EventSource, ev *event.Event) {
	if ev.Sender == s.login.UserID {
		return
	}
	mem := ev.Content.AsMember()
	switch(mem.Membership) {
	case "join":
		// Update member list
		// Write out
	case "invite":
		// Is this us? If so, notify; sending a /join will attach to this room. 
		// We may want to alias to something simple to type, or even
		// just handle the very last invite by default
	case "leave":
		// Update member list
		// Write out
	case "ban":
		// Is this us? If so, notify
	}
}

func (s *Session) message(src mautrix.EventSource, ev *event.Event) {
	if(ev.Sender == s.login.UserID) {
		return
	}
	room := s.rooms[ev.RoomID.String()]
	// We don't really want this, we lose messages, but we need a good name
	if room == "unknown" {
		return
	}
	mw, err := s.ctrl.MainWriter(room)
	if err != nil {
		return
	}

	fmt.Fprintf(mw, ev.Content.AsMessage().FormattedBody)
	mw.Close()
	//msg := ev.Content.AsMessage()
	//msg.FormattedBody
	// Check mtype, handle accordingly
	// Write out
	if e := s.client.MarkRead(ev.RoomID, ev.ID); e != nil {
		s.debug(ctlErr, e)
	}
}

func (s *Session) connect() error {
	switch s.Defaults.SSL {
	case "none":
	case "simple":
		var conn *tls.Conn
		var err error
		tlsConfig := http.DefaultTransport.(*http.Transport).TLSClientConfig
		s.client.Client = &http.Client{
			Transport: &http.Transport{
				DialTLS: func(network, addr string) (net.Conn, error) {
					conn, err = tls.Dial(network, addr, tlsConfig)
					return conn, err
				},
			},
		}
		s.debug(ctlSucceed, "set up TLS")
		return nil
	/*case "certificate":
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
		*/
	}

	return nil
}

func ctlLogging(ctl ctlItem, args ...interface{}) {
	l := log.New(os.Stdout, "matrixfs ", 0)
	switch ctl {
	case ctlRun:
		l.Printf("running")
	case ctlInfo:
		l.Printf("info: %v", args)
	case ctlStart:
		l.Printf("starting %s", args[0])
	case ctlSucceed:
		l.Printf("%s succeeded\n", args[0])
	case ctlInput:
		l.Printf("input: data=\"%s\" bufname=\"%s\"", args[0], args[1])
	case ctlMember:
		if m, ok := args[0].(*event.Event); ok {
			l.Printf("member name=\"%s\"", m.Sender)
		}
	case ctlMsg:
		l.Printf("message: \"%s\"", args[0])
	case ctlCommand:
		if m, ok := args[0].(*commander.Command); ok {
			l.Printf("command name=\"%s\" heading=\"%d\" sender=\"%s\" args=\"%s\" from=\"%s\"", m.Name, m.Heading, m.Sender, m.Args, m.From)
		}
	case ctlLogin:
		if m, ok := args[0].(*mautrix.RespLogin); ok {
			l.Printf("login success: userID=\"%s\" deviceID=\"%s\"", m.UserID, m.DeviceID)
		}
	case ctlLoginReq:
		if m, ok := args[0].(*mautrix.ReqLogin); ok {
			l.Printf("login attempt: user=\"%s\" pass=\"%s\" type=\"%s\" deviceID=\"%s\"", m.Identifier.User, m.Password, m.Type, m.DeviceID)
		}
	case ctlErr:
		if m, ok := args[0].(error); ok {
			l.Printf("error=\"%v\"\n", m.Error())
		}
	}
}
