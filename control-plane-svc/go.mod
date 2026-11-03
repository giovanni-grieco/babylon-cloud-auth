module control-plane-svc

go 1.27.3

require (
    common v0.0.0
    gorm.io/driver/postgres v1.6.3
	gorm.io/gorm v1.31.2
)

replace common => ../common
