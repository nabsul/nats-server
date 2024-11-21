package server

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const usrLookupReqSubj = "$SYS.REQ.USER.%s.CLAIMS.LOOKUP"
const DEFAULT_USER_FETCH_TIMEOUT = 1900 * time.Millisecond

func (s *Server) fetchUser(name string) string {
	s.mu.Unlock()
	timeout := DEFAULT_USER_FETCH_TIMEOUT
	if s == nil {
		s.Errorf("Server is nil: %q", ErrNoAccountResolver)
		return _EMPTY_
	}
	respC := make(chan []byte, 1)
	userLookupRequest := fmt.Sprintf(usrLookupReqSubj, name)
	s.Noticef("Fetching user %q with subject %q", name, userLookupRequest)
	s.mu.Lock()
	if s.sys == nil || s.sys.replies == nil {
		s.mu.Unlock()
		s.Errorf("eventing shut down")
		return _EMPTY_
	}
	// Resolver will wait for detected active servers to reply
	// before serving an error in case there weren't any found.
	expectedServers := len(s.sys.servers)
	replySubj := s.newRespInbox()
	replies := s.sys.replies

	s.Noticef("Setting up handler on reply subject %q", replySubj)

	// Store our handler.
	replies[replySubj] = func(sub *subscription, _ *client, _ *Account, subject, _ string, msg []byte) {
		var clone []byte
		isEmpty := len(msg) == 0
		if !isEmpty {
			clone = make([]byte, len(msg))
			copy(clone, msg)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		expectedServers--
		// Skip empty responses until getting all the available servers.
		if isEmpty && expectedServers > 0 {
			return
		}
		// Use the first valid response if there is still interest or
		// one of the empty responses to signal that it was not found.
		if _, ok := replies[replySubj]; ok {
			select {
			case respC <- clone:
			default:
			}
		}
	}
	s.Noticef("Sending internal message to %q", userLookupRequest)
	s.sendInternalMsg(userLookupRequest, replySubj, nil, []byte{})
	quit := s.quitCh
	s.mu.Unlock()
	var err error
	s.Noticef("Fetching user %q", name)
	var theJWT string
	select {
	case <-quit:
		s.Noticef("Fetching user %q failed due to shutdown", name)
		err = errors.New("fetching user jwt failed due to shutdown")
	case <-time.After(timeout):
		s.Noticef("Fetching user %q timed out", name)
		err = errors.New("fetching user jwt timed out")
	case m := <-respC:
		s.Noticef("Received response for user %q", string(m))
		if len(m) == 0 {
			err = errors.New("user jwt not found")
		} else {
			theJWT = string(m)
		}
	}
	s.mu.Lock()
	delete(replies, replySubj)
	s.mu.Unlock()
	close(respC)
	s.Noticef("Fetched user %q and received JWT %q", name, theJWT)

	if err != nil {
		s.Errorf("Error fetching user: %q", err)
		return _EMPTY_
	}

	if strings.HasPrefix(theJWT, "error:") {
		s.Errorf("Error fetching user: %q", theJWT)
		return _EMPTY_
	}

	return theJWT
}
