package call

import (
	"context"
	"fmt"
	"log"
)

// Manager handles the rooms.
type Manager interface {
	// Add append an item to the subscriptions list.
	AddPeer(context.Context, RoomID, UserID) (*Connection, error)
	// Notify the subscription rooms about a new event but removes
	// the give excluded IDS from the list.
	SendMessage(ctx context.Context, room RoomID, sender UserID, msg *Interaction) (*Room, error)
	// Remove removes an user from the given list.
	Finish(context.Context, RoomID, UserID) (*Room, error)
}

// CheckAccess function called before any event.
type CheckAccess func(context.Context, RoomID, UserID) error

// Event represent the
type Event func(context.Context, RoomID, UserID) error

type callManager struct {
	notifier     Notifier
	CheckNew     CheckAccess
	CheckMessage CheckAccess
	CheckFinish  CheckAccess
	AfterFinish  Event
}

// Option change the callManager default functionality.
type Option func(notifier *callManager)

// CheckNew is called to check if the room could be created.
func CheckNew(access CheckAccess) Option {
	return func(call *callManager) {
		call.CheckNew = access
	}
}

// CheckMessage checks if the user can send messages to the room.
func CheckMessage(access CheckAccess) Option {
	return func(call *callManager) {
		call.CheckMessage = access
	}
}

// CheckFinish check if the user can finish the room.
func CheckFinish(access CheckAccess) Option {
	return func(call *callManager) {
		call.CheckFinish = access
	}
}

// NewManager is the manager constructor.
func NewManager(notifier Notifier, ops ...Option) Manager {
	call := &callManager{notifier: notifier}
	for _, op := range ops {
		op(call)
	}
	return call
}

// AddPeer append an item to the subscriptions list.
func (r *callManager) AddPeer(ctx context.Context, rmID RoomID, uid UserID) (*Connection, error) {
	if r.CheckNew != nil {
		if err := r.CheckNew(ctx, rmID, uid); err != nil {
			return nil, err
		}
	}

	channel := make(chan *Interaction)
	connection := NewConnection(channel)
	uuu := string(uid)
	puuu := fmt.Sprintf("\"%s\"", uuu)

	_ = r.notifier.Notify(ctx, rmID, []UserID{uid}, &Interaction{NewPeer: &puuu})
	if err := r.notifier.Add(ctx, rmID, uid, connection); err != nil {
		// Cleanup.
		_ = r.notifier.Remove(ctx, rmID, uid, connection)
		return nil, err
	}

	go func() {
		<-ctx.Done()
		log.Printf("disconnecting peer %s from the room %s", uid, rmID)
		_ = r.notifier.Remove(ctx, rmID, uid, connection)
		_ = r.notifier.Notify(ctx, rmID, []UserID{uid}, &Interaction{Disconnected: &puuu})
	}()

	return connection, nil
}

// Notify the subscription rooms about a new event but removes
// the give excluded IDS from the list.
func (r *callManager) SendMessage(ctx context.Context, rmID RoomID, sender UserID, msg *Interaction) (*Room, error) {
	if r.CheckMessage != nil {
		if err := r.CheckMessage(ctx, rmID, sender); err != nil {
			return nil, err
		}
	}

	if err := r.notifier.Notify(ctx, rmID, []UserID{sender}, msg); err != nil {
		return nil, err
	}

	return NewRoom(rmID), nil
}

// Remove removes an user from the given list.
func (r *callManager) Finish(ctx context.Context, rmID RoomID, uid UserID) (*Room, error) {
	if r.CheckFinish != nil {
		if err := r.CheckFinish(ctx, rmID, uid); err != nil {
			return nil, err
		}
	}

	uuu := fmt.Sprintf("\"%s\"", rmID)
	err := r.notifier.Notify(ctx, rmID, []UserID{uid}, &Interaction{
		Finished: &uuu,
	})

	if err != nil {
		return nil, err
	}

	// TODO close all connections.

	return NewRoom(rmID), nil
}
