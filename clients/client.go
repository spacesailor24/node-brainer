package clients

type Client interface {
	Download() error
}
