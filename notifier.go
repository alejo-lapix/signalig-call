package call

import (
	"context"
	"github.com/google/uuid"
	"log"
	"sync"
)

// UserID represents the user identifier.
type UserID string

// RoomID represents the room identifier.
type RoomID string

// Notifier handles the channel communication.
type Notifier interface {
	// Add append an item to the subscriptions list.
	Add(context.Context, RoomID, UserID, *Connection) error
	// Notify the subscription rooms about a new event but removes
	// the give excluded IDS from the list.
	Notify(ctx context.Context, room RoomID, exclude []UserID, msg *Interaction) error
	// Remove removes an user from the given list.
	Remove(context.Context, RoomID, UserID, *Connection) error
}

// RoomHandler handles a room state.
type RoomHandler interface {
	// Add append an item to the subscriptions list.
	Add(context.Context, UserID, *Connection) error
	// Notify the subscription rooms about a new event but removes
	// the give excluded IDS from the list.
	Notify(ctx context.Context, exclude []UserID, msg *Interaction) error
	// Remove removes an user from the given list.
	Remove(context.Context, UserID, *Connection) error
	// Users return number of users that exists in the given room.
	Users(context.Context) (int, error)
}

// Connection holds the channel and peer info.
type Connection struct {
	ID      string
	Channel chan *Interaction
}

// Interaction is sent to the other peer messages.
type Interaction struct {
	Joined          *string `json:"joined"`
	NewPeer         *string `json:"newPeer"`
	NewOffer        *string `json:"newOffer"`
	NewAnswer       *string `json:"newAnswer"`
	NewIceCandidate *string `json:"newIceCandidate"`
	Finished        *string `json:"finished"`
	Disconnected    *string `json:"disconnected"`
}

// NewConnection is the connection constructor.
func NewConnection(channel chan *Interaction) *Connection {
	return &Connection{
		ID:      uuid.New().String(),
		Channel: channel,
	}
}

var _ Notifier = &center{}

type center struct {
	rooms map[RoomID]*Room
	mx    sync.Mutex
}

// NewCenter is the center constructor.
func NewCenter() Notifier {
	return &center{
		rooms: map[RoomID]*Room{},
	}
}

// Room represents the call.
type Room struct {
	id    RoomID
	mx    sync.Mutex
	users map[UserID]map[string]*Connection
}

// Peer represents the call participants.
type Peer struct {
	ID string
}

// ID holds the room identifier.
func (r *Room) ID() string {
	return string(r.id)
}

// Peers holds the room peers.
func (r *Room) Peers() []*Peer {
	r.mx.Lock()
	defer r.mx.Unlock()
	ids := make([]*Peer, len(r.users))
	for userID := range r.users {
		ids[len(ids)] = &Peer{ID: string(userID)}
	}
	return ids
}

// NewRoom is the room constructor.
func NewRoom(id RoomID) *Room {
	return &Room{
		id:    id,
		users: map[UserID]map[string]*Connection{},
	}
}

// Add append an item to the subscriptions list.
func (r *center) Add(ctx context.Context, rmID RoomID, uid UserID, connection *Connection) error {
	r.mx.Lock()
	defer r.mx.Unlock()

	room, ok := r.rooms[rmID]

	if !ok {
		r.rooms[rmID] = NewRoom(rmID)
		room = r.rooms[rmID]
		log.Printf("[ROOM] creating the room %s", rmID)
	}

	if err := room.Add(ctx, uid, connection); err != nil {
		if errRmv := room.Remove(ctx, uid, connection); errRmv != nil {
			log.Printf("[ROOM] error while removing a corrupted connection with the '%s' from the user '%s'", connection.ID, uid)
		}
		return err
	}

	return nil
}

// Notify the subscription rooms about a new event but removes
// the give excluded IDS from the list.
func (r *center) Notify(ctx context.Context, rmID RoomID, exclude []UserID, msg *Interaction) error {
	r.mx.Lock()
	defer r.mx.Unlock()

	room, ok := r.rooms[rmID]

	if !ok {
		return nil
	}

	return room.Notify(ctx, exclude, msg)
}

// Remove removes an user from the given list.
func (r *center) Remove(ctx context.Context, rmID RoomID, uid UserID, connection *Connection) error {
	r.mx.Lock()
	defer r.mx.Unlock()

	room, ok := r.rooms[rmID]

	if !ok {
		return nil
	}

	if err := room.Remove(ctx, uid, connection); err != nil {
		return err
	}

	totalUsers, err := room.Users(ctx)
	if err != nil {
		log.Printf("[ROOM] error while checking if the room could be deleted %s", room.id)
		return nil
	}

	if totalUsers == 0 {
		delete(r.rooms, rmID)
		log.Printf("[ROOM] deleted %s", rmID)
	} else {
		log.Printf("[ROOM] could not delete the room because already has peers (%d)", totalUsers)
	}

	return nil
}

func (r *center) Room(ctx context.Context, rmID RoomID) (*Room, error) {
	r.mx.Lock()
	defer r.mx.Unlock()

	if room, ok := r.rooms[rmID]; ok {
		return room, nil
	}

	return nil, nil
}

// Add append an item to the subscriptions list.
func (r *Room) Add(_ context.Context, uid UserID, connection *Connection) error {
	r.mx.Lock()
	defer r.mx.Unlock()

	connections, ok := r.users[uid]

	if !ok {
		r.users[uid] = map[string]*Connection{}
		connections = r.users[uid]
		log.Printf("[ROOM] creating the user '%s' connections map in the room %s", uid, r.id)
	}

	for _, connection := range connections {
		close(connection.Channel)
		log.Printf("[ROOM] removed connection '%s' from the users '%s' in the room '%s'", connection.ID, uid, r.id)
	}

	connections[connection.ID] = connection
	log.Printf("[ROOM] user %s added to the room %s", uid, r.id)

	return nil
}

// Notify the subscription rooms about a new event but removes
// the give excluded IDS from the list.
func (r *Room) Notify(ctx context.Context, exclude []UserID, msg *Interaction) error {
	var excludeMap = make(map[UserID]bool, 0)

	for _, uid := range exclude {
		excludeMap[uid] = true
	}

	r.mx.Lock()
	defer r.mx.Unlock()

	for uid, users := range r.users {
		if _, ok := excludeMap[uid]; !ok {
			for _, connection := range users {
				connection.Channel <- msg
				if len(exclude) != 0 {
					log.Printf("[ROOM] message from %s to %s", exclude[0], uid)
				}
			}
		}
	}

	return nil
}

// Remove removes an user from the given list.
func (r *Room) Remove(ctx context.Context, uid UserID, connection *Connection) error {
	r.mx.Lock()
	defer r.mx.Unlock()

	if userConnections, hasConnections := r.users[uid]; hasConnections {
		if _, ok := userConnections[connection.ID]; ok {
			delete(userConnections, connection.ID)
			log.Printf("[ROOM] deleted user's '%s' connection '%s' in the room '%s'", uid, connection.ID, r.id)

			if len(userConnections) == 0 {
				delete(r.users, uid)
			} else {
				log.Printf("[ROOM] the user '%s' already has more connection in the room '%s' (%d) could not delete the user connections map", uid, r.id, len(userConnections))
			}
		} else {
			log.Printf("[ROOM] the user '%s' does not have a connection in the room '%s' with the id '%s'", uid, r.id, connection.ID)
		}
	} else {
		log.Printf("[ROOM] the user '%s' does not have connections in the room '%s'", uid, r.id)
	}

	return nil
}

// Users tells the total number of peers in the room.
func (r *Room) Users(ctx context.Context) (int, error) {
	return len(r.users), nil
}
