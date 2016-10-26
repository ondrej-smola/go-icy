package icy

type Headers []*Header

func (headers *Headers) Add(key, value string) {
	*headers = append(*headers, &Header{
		Key:   key,
		Value: value,
	})
}

func (headers Headers) Get(key string) string {
	for _, header := range headers {
		if header.Key == key {
			return header.Value
		}
	}

	return ""
}

type Header struct {
	Key   string
	Value string
}
