package chinese_location_code

type Basic struct {
	Name string
}

func (b *Basic) SetName(name string) {
	b.Name = name
}

type Node interface {
	SetName(string)
}
