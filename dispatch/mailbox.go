package dispatch

import (
	"github.com/go-akka/akka"
	"github.com/go-akka/akka/pkg/lfqueue"
	"sync/atomic"
)

const (
	MailboxStatusOpen                 int32 = 0
	MailboxStatusClosed               int32 = 1
	MailboxStatusScheduled            int32 = 2
	MailboxStatusShouldScheduleMask   int32 = 3
	MailboxStatusShouldNotProcessMask int32 = ^2
	MailboxStatusSuspendMask          int32 = ^3
	MailboxStatusSuspendUnit          int32 = 4
	MailboxStatusSuspendAwaitTask     int32 = ^4
)

type Mailbox struct {
	actor akka.Cell

	invoker akka.MessageInvoker

	messageQueue akka.MessageQueue
	dispatcher   akka.MessageDispatcher

	deadLetterMailbox DeadLetterMailbox

	systemMailbox *lfqueue.LockfreeQueue

	status int32
}

func NewMailbox(messageQueue akka.MessageQueue) akka.Mailbox {
	return &Mailbox{
		messageQueue:  messageQueue,
		systemMailbox: lfqueue.NewLockfreeQueue(),
	}
}

func (p *Mailbox) SetActor(actor akka.Cell) {
	p.actor = actor
}

func (p *Mailbox) Dispatcher() akka.MessageDispatcher {
	return p.actor.Dispatcher()
}

func (p *Mailbox) MessageQueue() akka.MessageQueue {
	return p.messageQueue
}

func (p *Mailbox) Enqueue(receiver akka.ActorRef, envelope akka.Envelope) (err error) {
	return p.messageQueue.Enqueue(receiver, envelope)
}

func (p *Mailbox) Dequeue() (envelope akka.Envelope, ok bool) {
	return p.messageQueue.Dequeue()
}

func (p *Mailbox) NumberOfMessages() int {
	return p.messageQueue.NumberOfMessages()
}

func (p *Mailbox) HasMessages() bool {
	return p.messageQueue.HasMessages()
}

func (p *Mailbox) CleanUp(owner akka.ActorRef, deadLetters akka.MessageQueue) (err error) {

	if p.messageQueue != nil {
		p.messageQueue.CleanUp(p.actor.Self(), p.deadLetterMailbox.MessageQueue())
	}

	return
}

func (p *Mailbox) Run() {
	defer func() {
		p.setAsIdle()
		p.Dispatcher().RegisterForExecution(p, false, false)
	}()

	if !p.isClosed() {
		p.processAllSystemMessages()
		//TODO: add timeout
		p.processMailbox(p.max(1, p.Dispatcher().Throughput()))
	}
}

func (p *Mailbox) processAllSystemMessages() {

	return
}

func (p *Mailbox) processMailbox(left int) {

	for p.shouldProcessMessage() {
		next, ok := p.Dequeue()
		if !ok {
			return
		}

		p.invoker.Invoke(next)
		p.processAllSystemMessages()

		if left > 1 {
			left--
			continue
		}

		break
	}

	return
}

func (p *Mailbox) currentStatus() int32 {
	return atomic.LoadInt32(&p.status)
}

func (p *Mailbox) updateStatus(oldStatus int32, newStatus int32) bool {
	return atomic.CompareAndSwapInt32(&p.status, oldStatus, newStatus)
}

func (p *Mailbox) setStatus(newStatus int32) {
	atomic.StoreInt32(&p.status, newStatus)
}

func (p *Mailbox) shouldProcessMessage() bool {
	return (p.currentStatus() & MailboxStatusShouldNotProcessMask) == 0
}

func (p *Mailbox) isSuspended() bool {
	return (p.currentStatus() & MailboxStatusSuspendMask) != 0
}

func (p *Mailbox) isClosed() bool {
	return p.currentStatus() == MailboxStatusClosed
}

func (p *Mailbox) isScheduled() bool {
	return (p.currentStatus() & MailboxStatusScheduled) != 0
}

func (p *Mailbox) resume() bool {
	status := p.currentStatus()
	if status == MailboxStatusClosed {
		p.setStatus(MailboxStatusClosed)
		return false
	}

	next := status
	if status >= MailboxStatusSuspendUnit {
		next = status - MailboxStatusSuspendUnit
	}

	if p.updateStatus(status, next) {
		return next < MailboxStatusSuspendUnit
	}

	return p.resume()
}

func (p *Mailbox) suspend() bool {
	status := p.currentStatus()
	if status == MailboxStatusClosed {
		p.setStatus(MailboxStatusClosed)
		return false
	}

	if p.updateStatus(status, status+MailboxStatusSuspendUnit) {
		return status < MailboxStatusSuspendUnit
	}

	return p.suspend()
}

func (p *Mailbox) becomeClosed() bool {
	status := p.currentStatus()
	if status == MailboxStatusClosed {
		p.setStatus(MailboxStatusClosed)
		return false
	}

	return p.updateStatus(status, MailboxStatusClosed) || p.becomeClosed()
}

func (p *Mailbox) setAsIdle() bool {
	for {
		status := p.currentStatus()
		if p.updateStatus(status, status&^MailboxStatusScheduled) {
			return true
		}
	}
}

func (p *Mailbox) max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
