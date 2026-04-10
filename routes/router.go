package routes

import (
	"rentflow-api/config"
	"rentflow-api/controllers"
	"rentflow-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(r *gin.Engine) {
	// เปิดใช้งาน CORS middleware
	r.Use(middleware.CORSMiddleware())
	// ใส่ database instance ลง context ให้ทุก request ใช้งานได้
	r.Use(middleware.DBMiddleware(config.DB))

	// กลุ่ม route สำหรับระบบ Authentication เช่น ลงทะเบียน, เข้าสู่ระบบ
	auth := r.Group("/auth")
	{
		auth.POST("/login", controllers.Login)                                                                      // POST /auth/login // เข้าสู่ระบบ รับ token
		auth.GET("/profile", middleware.JWTAuthMiddleware(), controllers.Profile)                                   // GET /auth/profile // ดูข้อมูลโปรไฟล์ผู้ใช้ (ต้อง login)
		auth.PUT("/profile", middleware.JWTAuthMiddleware(), controllers.UpdateOwnProfile)                          // PUT /auth/profile // อัปเดตข้อมูลโปรไฟล์ผู้ใช้ (ต้อง login)
		auth.POST("/refresh", middleware.JWTAuthMiddleware(), controllers.RefreshToken)                             // POST /auth/refresh // รีเฟรช access token (ต้อง login)
		auth.POST("/forgot-password", controllers.ForgotPassword)                                                   // POST /auth/forgot-password // ขอรีเซ็ตรหัสผ่าน
		auth.PUT("/change-password", middleware.JWTAuthMiddleware(), controllers.ChangeOwnPassword)                 // PUT /auth/change-password // เปลี่ยนรหัสผ่านตัวเอง (ต้อง login)
		auth.PUT("/users/:id/change-password", middleware.JWTAuthMiddleware(), controllers.AdminChangeUserPassword) // PUT /auth/users/:id/change-password // admin เปลี่ยนรหัสผ่านผู้ใช้อื่น (ต้อง login)
	}

	// สมัครพนักงานใหม่ (ไม่ต้อง login)
	// r.POST("/api/employees/register", controllers.HandleEmployeeRegister) // POST /api/employees/register // สมัครพนักงานใหม่ (อัปโหลดไฟล์แนบได้)

	// กลุ่ม route สำหรับ API ที่ต้อง login ทุกครั้ง
	api := r.Group("/api")
	api.Use(middleware.JWTAuthMiddleware()) // ใช้ JWT ตรวจสอบ token ทุก route ในกลุ่มนี้
	{
		// จัดการสินค้าตามหมวดหมู่

		// จัดการแดชบอร์ด
		

		// จัดการคำสั่งซื้อ

		// จัดการแคชสินค้า
		api.POST("/auth/refresh", controllers.RefreshAccessToken)                           // กลุ่มจัดการผู้ใช้ (จำกัดสิทธิ์ admin หรือ superadmin เท่านั้น)

		// จัดการผู้ใช้
		usersGroup := api.Group("/users")              // กลุ่มจัดการผู้ใช้ (จำกัดสิทธิ์ admin หรือ superadmin เท่านั้น)
		usersGroup.Use(middleware.AdminOrSuperAdmin()) // middleware ตรวจสอบ role

		{
			// CRUD ผู้ใช้
			usersGroup.GET("", controllers.GetUsers)          // GET /api/users // ดึงรายชื่อผู้ใช้ทั้งหมด
			usersGroup.PUT("/:gmail", controllers.UpdateUser)    // PUT /api/users/:gmail // อัปเดตข้อมูลผู้ใช้ตาม gmail
			usersGroup.DELETE("/gmail/:gmail", controllers.DeleteUser) // DELETE /api/users/gmail/:gmail // ลบผู้ใช้ตาม gmail

			// จัดการผู้ใช้ที่ยังรออนุมัติ (สร้างคำขอและยืนยัน OTP)
			usersGroup.POST("/requests", controllers.CreateUserRequestByAdmin)     // POST /api/users/requests // สร้างคำขอสร้างผู้ใช้ใหม่ (รอ OTP)
			usersGroup.POST("/requests/verify", controllers.VerifyAndActivateUser) // POST /api/users/requests/verify // ยืนยัน OTP เพื่อสร้างผู้ใช้จริง
		}
	}
}