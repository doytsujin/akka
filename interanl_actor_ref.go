package akka

type InternalActorRef interface {
	ActorRefScope
	ActorRef

	Provider() ActorRefProvider

	String() string
	Parent() InternalActorRef
	GetChild(names ...string) InternalActorRef

	Start()
	Resume(err error)
	Suspend()
	Restart(err error)
	Stop()
}
