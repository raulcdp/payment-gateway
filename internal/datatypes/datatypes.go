package datatypes

import "encoding/json"

func StructToMap(obj any) (map[string]any, error) {
	b, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}

	var values map[string]interface{}
	err = json.Unmarshal(b, &values)
	if err != nil {
		return nil, err
	}
	return values, nil
}
