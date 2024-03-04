package clients

type Client interface {
	Download() error
	Start() error
	Stop() error
	Logs() error
}
