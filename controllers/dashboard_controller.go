package controllers

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"rentflow-api/config"
	"rentflow-api/models"
)

var dashboardProductTables = []string{
	"dried_food",
	"soft_drink",
	"stationery",
	"fresh_food",
	"snack",
}

var tableNameMap = map[string]string{
	"dried_food": "ประเภทแห้ง",
	"soft_drink": "ประเภทเครื่องดื่ม",
	"stationery": "ประเภทเครื่องเขียน",
	"fresh_food": "ประเภทแช่แข็ง",
	"snack":      "ประเภทขนม",
}

func GetTotalProducts(c *gin.Context) {
	result := make(map[string]int64)
	var grandTotal int64 = 0

	for _, table := range dashboardProductTables {
		var sum sql.NullInt64
		if err := config.DB.Table(table).Select("SUM(stock)").Scan(&sum).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"ข้อความผิดพลาด": "ไม่สามารถดึงข้อมูลได้จากตาราง " + table,
				"error": err.Error(),
			})
			return
		}

		total := int64(0)
		if sum.Valid {
			total = sum.Int64
		}

		htName := tableNameMap[table]
		result[htName] = total
		grandTotal += total
	}

	c.JSON(http.StatusOK, gin.H{
		"จำนวนสินค้าตามประเภท": result,
		"จำนวนสินค้าทั้งหมด":   grandTotal,
	})
}

func GetLowStockProducts(c *gin.Context) {
	lowStock := make(map[string][]models.ProductDashboard)
	totalLowCount := int64(0)

	for _, table := range dashboardProductTables {
		var products []models.ProductDashboard
		if err := config.DB.Table(table).
			Select("id, product_name, barcode, stock, reorder_level").
			Where("stock > 0 AND stock <= reorder_level").
			Find(&products).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"ข้อความผิดพลาด": "ไม่สามารถดึงข้อมูลได้จากตาราง " + table,
				"error": err.Error(),
			})
			return
		}

		if products == nil {
			products = []models.ProductDashboard{}
		}

		htName := tableNameMap[table]
		lowStock[htName] = products

		var count int64
		if err := config.DB.Table(table).
			Where("stock > 0 AND stock <= reorder_level").Count(&count).Error; err == nil {
			totalLowCount += count
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"สินค้าใกล้หมดสต๊อก": lowStock,
		"จำนวนรวมทั้งหมด":    totalLowCount,
	})
}

func GetOutOfStockProducts(c *gin.Context) {
	outStock := make(map[string][]models.ProductDashboard)
	totalOutCount := int64(0)

	for _, table := range dashboardProductTables {
		var products []models.ProductDashboard
		if err := config.DB.Table(table).
			Select("id, product_name, barcode, stock").
			Where("stock = 0").
			Find(&products).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"ข้อความผิดพลาด": "ไม่สามารถดึงข้อมูลได้จากตาราง " + table,
				"error": err.Error(),
			})
			return
		}

		if products == nil {
			products = []models.ProductDashboard{}
		}

		htName := tableNameMap[table]
		outStock[htName] = products

		var count int64
		if err := config.DB.Table(table).Where("stock = 0").Count(&count).Error; err == nil {
			totalOutCount += count
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"สินค้าหมดสต๊อก":  outStock,
		"จำนวนรวมทั้งหมด": totalOutCount,
	})
}

func GetMonthlySalesSummary(c *gin.Context) {
	type SalesSummary struct {
		Year       int     `json:"year"`
		Month      string  `json:"month"`
		TotalSales float64 `json:"total_sales"`
	}

	var summaries []SalesSummary

	query := `
    SELECT 
        year,
        CASE month
            WHEN 1 THEN 'ม.ค.'
            WHEN 2 THEN 'ก.พ.'
            WHEN 3 THEN 'มี.ค.'
            WHEN 4 THEN 'เม.ย.'
            WHEN 5 THEN 'พ.ค.'
            WHEN 6 THEN 'มิ.ย.'
            WHEN 7 THEN 'ก.ค.'
            WHEN 8 THEN 'ส.ค.'
            WHEN 9 THEN 'ก.ย.'
            WHEN 10 THEN 'ต.ค.'
            WHEN 11 THEN 'พ.ย.'
            WHEN 12 THEN 'ธ.ค.'
        END AS month,
        total_sales
    FROM (
        SELECT 
            YEAR(sold_at) AS year,
            MONTH(sold_at) AS month,
            SUM(price * quantity) AS total_sales
        FROM sales_today
        GROUP BY YEAR(sold_at), MONTH(sold_at)
    ) AS t
    ORDER BY year, month
`

	rows, err := config.DB.Raw(query).Rows()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"ข้อความผิดพลาด": "ไม่สามารถดึงข้อมูลยอดขายรายเดือนทั้งหมดได้",
			"error": err.Error(),
		})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var s SalesSummary
		if err := rows.Scan(&s.Year, &s.Month, &s.TotalSales); err == nil {
			summaries = append(summaries, s)
		}
	}

	var totalAll float64 = 0
	for _, s := range summaries {
		totalAll += s.TotalSales
	}

	c.JSON(http.StatusOK, gin.H{
		"ยอดขายรายเดือนทั้งหมด": summaries,
		"ยอดขายรวมทั้งหมด":      totalAll,
	})
}

func GetWeeklySalesCurrentMonth(c *gin.Context) {
	type SalesSummary struct {
		Week       int     `json:"week"`
		WeekLabel  string  `json:"week_label"`
		TotalSales float64 `json:"total_sales"`
	}

	var summaries []SalesSummary

	query := `
	SELECT
		week_in_month AS week,
		CONCAT('สัปดาห์ที่ ', week_in_month) AS week_label,
		SUM(price * quantity) AS total_sales
	FROM (
		SELECT
			price,
			quantity,
			FLOOR((DAY(sold_at) - 1) / 7) + 1 AS week_in_month
		FROM sales_today
		WHERE
			MONTH(sold_at) = MONTH(CURRENT_DATE())
			AND YEAR(sold_at) = YEAR(CURRENT_DATE())
	) t
	GROUP BY week_in_month
	ORDER BY week_in_month
	`

	rows, err := config.DB.Raw(query).Rows()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"ข้อความผิดพลาด": "ไม่สามารถดึงยอดขายรายสัปดาห์ของเดือนปัจจุบันได้",
			"error": err.Error(),
		})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var s SalesSummary
		if err := rows.Scan(&s.Week, &s.WeekLabel, &s.TotalSales); err == nil {
			summaries = append(summaries, s)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"ยอดขายรายสัปดาห์เดือนปัจจุบัน": summaries,
	})
}

func GetTopSellingProductsCurrentMonth(c *gin.Context) {
	type TopProduct struct {
		ProductName string `json:"product_name"`
		ImageURL    string `json:"image_url"`
	}

	var products []TopProduct

	query := `
	SELECT
		product_name,
		image_url
	FROM sales_today
	WHERE
		MONTH(sold_at) = MONTH(CURRENT_DATE())
		AND YEAR(sold_at) = YEAR(CURRENT_DATE())
	GROUP BY product_name, image_url
	ORDER BY SUM(quantity) DESC
	LIMIT 3
	`

	rows, err := config.DB.Raw(query).Rows()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"ข้อความผิดพลาด": "ไม่สามารถดึงข้อมูลสินค้าขายดีของเดือนปัจจุบันได้",
			"error": err.Error(),
		})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var p TopProduct
		if err := rows.Scan(&p.ProductName, &p.ImageURL); err == nil {
			products = append(products, p)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"สินค้าขายดีเดือนปัจจุบัน": products,
	})
}
