package session

import (
	//"github.com/altid/libs/service/commander"
	"git.sr.ht/~f4814n/matrix"
)

func (s *Session) handle() error {
	for event := range s.events {
		switch v := event.(type) {
		case matrix.RoomEvent:
			if v.Type == "m.room.message" {
				;
			}
		}
	}

	return nil
}
