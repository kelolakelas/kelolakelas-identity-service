package domain

import "testing"

func TestUpdateTenantLocationRequestValidate(t *testing.T) {
	latitude, longitude := 1.0, 2.0
	invalidLatitude, invalidLongitude := 91.0, 181.0
	tests := []struct {
		name    string
		request UpdateTenantLocationRequest
		wantErr bool
	}{
		{name: "coordinates valid", request: UpdateTenantLocationRequest{Address: "A", Latitude: &latitude, Longitude: &longitude}},
		{name: "latitude missing", request: UpdateTenantLocationRequest{Address: "A", Longitude: &longitude}, wantErr: true},
		{name: "longitude missing", request: UpdateTenantLocationRequest{Address: "A", Latitude: &latitude}, wantErr: true},
		{name: "latitude out of range", request: UpdateTenantLocationRequest{Address: "A", Latitude: &invalidLatitude, Longitude: &longitude}, wantErr: true},
		{name: "longitude out of range", request: UpdateTenantLocationRequest{Address: "A", Latitude: &latitude, Longitude: &invalidLongitude}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if (test.request.Validate() != nil) != test.wantErr {
				t.Fatalf("Validate() error mismatch")
			}
		})
	}
}
