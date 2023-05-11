package session

import (
	"github.com/altid/libs/service/commander"
	"git.sr.ht/~f4814n/matrix"
)

func handler(s *Session) {
	for event := range s.events {
		switch v := event.(type) {
		case matrix.RoomEvent:
			
		}
	}
}
