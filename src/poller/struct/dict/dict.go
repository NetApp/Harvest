
package dict


type Dict struct {
	dict map[string]string
}

func New() *Dict {
	d := Dict{}
	d.dict = make(map[string]string)
	return &d
}

func (d *Dict) Set(key, val string) {
	d.dict[key] = val
}

func (d *Dict) Delete(key string) {
	delete(d.dict, key)
}

func (d *Dict) Get(key string) string {
	if value, has := d.dict[key]; has {
		return value
	}
	return ""
}

func (d *Dict) Pop(key string) string {
	if value, has := d.GetHas(key); has {
		d.Delete(key)
		return value
	}
	return ""
}

func (d *Dict) GetHas(key string) (string, bool) {
	value, has := d.dict[key]
	return value, has
}

func (d *Dict) Has(key string) bool {
	_, has := d.dict[key]
	return has
}

func (d *Dict) Iter() map[string]string {
	return d.dict
}

func (d *Dict) Keys() []string {
	keys := make([]string, len(d.dict))
	for k, _ := range d.dict {
		keys = append(keys, k)
	}
	return keys
}

func (d *Dict) String() string {
	s := ""
	for k, v := range d.dict {
		s += k + "=" + v + "\n"
	}
	return s
}

func (d *Dict) Values() []string {
	values := make([]string, len(d.dict))
	for _, v := range d.dict {
		values = append(values, v)
	}
	return values
}

func (d *Dict) IsEmpty() bool {
	return len(d.dict) == 0
}

func (d *Dict) Size() int {
	return len(d.dict)
}