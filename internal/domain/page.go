package domain

type Page struct {
	Page int `json:"page"`
	Size int `json:"size"`
}

func (p Page) Normalize() Page {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.Size < 1 {
		p.Size = 20
	}
	if p.Size > 200 {
		p.Size = 200
	}
	return p
}

func (p Page) Offset() int {
	p = p.Normalize()
	return (p.Page - 1) * p.Size
}
