
type ServiceLayoutDB []ServiceLayoutDBEntry
type ServiceLayoutDBEntry struct {
	ServiceComponentName       string   `json:"ServiceComponentName"`
	ServiceComponentDeviceList []string `json:"ServiceComponentDeviceList"`
}

func ServiceLayoutConstruct(ServiceComponents ServiceComponents, DeviceDataDB DeviceDataDB, ServiceLayoutDB *ServiceLayoutDB) {
	for _, ServiceComponent := range ServiceComponents {
		DeviceList := []string{}
		var ServiceLayoutDBEntry ServiceLayoutDBEntry
		for _, DeviceDataDBEntry := range DeviceDataDB {
			if CheckComponentKeys(ServiceComponent.ComponentKeys, DeviceDataDBEntry.DeviceData) {
				DeviceList = append(DeviceList, DeviceDataDBEntry.DeviceName)
			}
		}
		ServiceLayoutDBEntry.ServiceComponentName = ServiceComponent.ComponentName
		ServiceLayoutDBEntry.ServiceComponentDeviceList = DeviceList
		*ServiceLayoutDB = append(*ServiceLayoutDB, ServiceLayoutDBEntry)
	}
}