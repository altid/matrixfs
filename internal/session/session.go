package session

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/altid/libs/config/types"
	"github.com/altid/libs/markup"
	"github.com/altid/libs/service/commander"
	"github.com/altid/libs/service/controller"
	"github.com/matrix-org/gomatrix"
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
	ctlRun
	ctlErr
)

type Session struct {
	Defaults	*Defaults
	Verbose		bool
	Ctx			context.Context
	cancel		context.CancelFunc
	client		*gomatrix.Client
	ctrl		controller.Controller
	rooms		map[string]string
	login		*gomatrix.RespLogin
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
	s.debug = func(ctlItem, ...interface{}) {}
	if s.client, err = gomatrix.NewClient(s.Defaults.Address, "", ""); err != nil {
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
	// 
	return nil
}

func (s *Session) Start(c controller.Controller) error {
	var e error
	s.ctrl = c
	s.debug(ctlSucceed, "connection")
	// If we're a guest user, create a guest session
	if s.Defaults.User == "guest" {
		var resp *gomatrix.RespRegister
		s.debug(ctlStart, "registration")
		reg := &gomatrix.ReqRegister{
			Username: s.Defaults.User,
			Password: string(s.Defaults.Auth),
			DeviceID: "altid/matrixfs - guest",
		}
		if resp, _, e = s.client.RegisterGuest(reg); e != nil {
			return e
		}
		s.login.DeviceID = resp.DeviceID
		s.login.HomeServer = resp.HomeServer
		s.login.UserID = resp.UserID
		s.login.AccessToken = resp.AccessToken
	} else {
		req := &gomatrix.ReqLogin{
			Type:	 	"m.login.password",
			Password:	string(s.Defaults.Auth),
			User:	 	s.Defaults.User,
			DeviceID:	"altid/matrixfs",
		}
		s.debug(ctlLoginReq, req)
		if s.login, e = s.client.Login(req); e != nil {
			return e
		}
	}
	s.debug(ctlLogin, s.login)
	s.client.SetCredentials(s.login.UserID, s.login.AccessToken)
	// Sync handlers for the message types we need
	sync := s.client.Syncer.(*gomatrix.DefaultSyncer)
	sync.OnEventType("m.room.redaction", s.redaction)
	sync.OnEventType("m.room.message", s.message)
	sync.OnEventType("m.room.member", s.member)
	sync.OnEventType("m.room.avatar", s.avatar)
	sync.OnEventType("m.room.name", s.name)
	sync.OnEventType("m.canonical.alias", s.name)
	sync.OnEventType("m.room.topic", s.title)
	sync.OnEventType("m.room.create", s.create)
	// Call sync in a loop, check if we're done
	// Sync will call our event callbacks for us and keep up to date with the server
	s.debug(ctlSucceed, "Add sync events")
	ec := make(chan error)
	go func(ec chan error) {
		ec <- s.client.Sync()
	}(ec)
	for {
		select {
		// Ensure we are checking our main context as well
		case <-s.Ctx.Done():
			return nil
		case e = <-ec:
			// TODO: Handle err appropriately
			s.debug(ctlErr, e)
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

func (s *Session) title(ev *gomatrix.Event) {
	if s.rooms[ev.RoomID] == "unknown" {
		log.Fatal("Title event before room name event")
	}
	title, err := s.ctrl.TitleWriter(s.rooms[ev.RoomID])
	if err != nil {
		s.debug(ctlErr, err)
		return
	}
	if t, ok := ev.Content["topic"].(string); ok {
		fmt.Fprint(title, t)
	}
}

func (s *Session) create(ev *gomatrix.Event) {
	// We don't have the room name available here
	// Set to unknown, and try to find out before we log any real data
	if room, ok := s.rooms[ev.RoomID]; ok {
		if room == "unknown" {
			log.Fatal("Multiple create events, unexpected")
		}
	}
	s.rooms[ev.RoomID] = "unknown"
	//if e := s.ctrl.CreateBuffer(ev.RoomID); e != nil {
	//	s.debug(ctlErr, e)
	//}
}

func (s *Session) name(ev *gomatrix.Event) {
	switch(ev.Type) {
	case "m.room.name":
		if name, ok := ev.Content["name"].(string); ok {
			s.rooms[ev.RoomID] = name
		}
	case "m.canonical.alias":
		// Only set the alias if we don't have a good name
		if name, ok := ev.Content["alias"].(string); ok {
			if s.rooms[ev.RoomID] != "unknown" {
				s.rooms[ev.RoomID] = name
			}
		}
	}
}

func (s *Session) avatar(ev *gomatrix.Event) {
	// TODO: Build our title with an image instead
}

func (s *Session) redaction(ev *gomatrix.Event) {
	if(ev.Sender == s.login.UserID) {
		return
	}
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
	if e := s.client.MarkRead(ev.RoomID, ev.ID); e != nil {
		s.debug(ctlErr, e)
	}
}

func (s *Session) member(ev *gomatrix.Event) {
	switch(ev.Content["membership"]) {
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

func (s *Session) message(ev *gomatrix.Event) {
	if(ev.Sender == s.login.UserID) {
		return
	}
	// Check mtype, handle accordingly
	// Write out
	if e := s.client.MarkRead(ev.RoomID, ev.ID); e != nil {
		s.debug(ctlErr, e)
	}
}

func ctlLogging(ctl ctlItem, args ...interface{}) {
	l := log.New(os.Stdout, "matrixfs ", 0)
	switch ctl {
	case ctlRun:
		l.Printf("running")
	case ctlStart:
		l.Printf("starting %s", args[0])
	case ctlSucceed:
		l.Printf("%s succeeded\n", args[0])
	case ctlInput:
		l.Printf("input: data=\"%s\" bufname=\"%s\"", args[0], args[1])
	case ctlCommand:
		if m, ok := args[0].(*commander.Command); ok {
			l.Printf("command name=\"%s\" heading=\"%d\" sender=\"%s\" args=\"%s\" from=\"%s\"", m.Name, m.Heading, m.Sender, m.Args, m.From)
		}
	case ctlLogin:
		if m, ok := args[0].(*gomatrix.RespLogin); ok {
			l.Printf("login success: userID=\"%s\" homeserver=\"%s\" deviceID=\"%s\"", m.UserID, m.HomeServer, m.DeviceID)
		}
	case ctlLoginReq:
		if m, ok := args[0].(*gomatrix.ReqLogin); ok {
			l.Printf("login attempt: user=\"%s\" pass=\"%s\" type=\"%s\" deviceID=\"%s\"", m.User, m.Password, m.Type, m.DeviceID)
		}
	case ctlErr:
		if m, ok := args[0].(error); ok {
			l.Printf("error=\"%v\"\n", m.Error())
		}
	}
}
