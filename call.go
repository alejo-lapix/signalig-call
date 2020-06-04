package call

import (
	"context"
	"log"
)

type Manager interface {
	// Add append an item to the subscriptions list.
	AddPeer(context.Context, RoomID, UserID) (*Connection, error)
	// Notify the subscription rooms about a new event but removes
	// the give excluded IDS from the list.
	SendMessage(ctx context.Context, room RoomID, sender UserID, msg *Interaction) (*Room, error)
	// Remove removes an user from the given list.
	Finish(context.Context, RoomID, UserID) (*Room, error)
}

type CheckAccess func(context.Context, RoomID, UserID) error

type Call struct {
	notifier     Notifier
	CheckNew     CheckAccess
	CheckMessage CheckAccess
	CheckFinish  CheckAccess
}

type Option func(notifier *Call)

func CheckNew(access CheckAccess) Option {
	return func(call *Call) {
		call.CheckNew = access
	}
}

func CheckMessage(access CheckAccess) Option {
	return func(call *Call) {
		call.CheckMessage = access
	}
}

func CheckFinish(access CheckAccess) Option {
	return func(call *Call) {
		call.CheckFinish = access
	}
}

func NewCall(notifier Notifier, ops ...Option) *Call {
	call := &Call{notifier: notifier}
	for _, op := range ops {
		op(call)
	}
	return call
}

// Add append an item to the subscriptions list.
func (r *Call) AddPeer(ctx context.Context, rmID RoomID, uid UserID) (*Connection, error) {
	if r.CheckNew != nil {
		if err := r.CheckNew(ctx, rmID, uid); err != nil {
			return nil, err
		}
	}

	channel := make(chan *Interaction)
	connection := NewConnection(channel)
	uuu := string(uid)

	r.notifier.Notify(ctx, rmID, []UserID{uid}, &Interaction{NewPeer: &uuu})
	if err := r.notifier.Add(ctx, rmID, uid, connection); err != nil {
		// Cleanup.
		r.notifier.Remove(ctx, rmID, uid, connection)
		return nil, err
	}

	go func() {
		<-ctx.Done()
		log.Printf("disconnecting peer %s from the room %s", uid, rmID)
		r.notifier.Remove(ctx, rmID, uid, connection)
		r.notifier.Notify(ctx, rmID, []UserID{uid}, &Interaction{Disconnected: &uuu})
	}()

	return connection, nil
}

// Notify the subscription rooms about a new event but removes
// the give excluded IDS from the list.
func (r *Call) SendMessage(ctx context.Context, rmID RoomID, sender UserID, msg *Interaction) (*Room, error) {
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
func (r *Call) Finish(ctx context.Context, rmID RoomID, uid UserID) (*Room, error) {
	if r.CheckFinish != nil {
		if err := r.CheckFinish(ctx, rmID, uid); err != nil {
			return nil, err
		}
	}

	uuu := string(rmID)
	err := r.notifier.Notify(ctx, rmID, []UserID{uid}, &Interaction{
		Finished: &uuu,
	})

	if err != nil {
		return nil, err
	}

	// TODO close all connections.

	return NewRoom(rmID), nil
}
