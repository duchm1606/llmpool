package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

func bindJSONAndValidate(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		if validationErrs, ok := err.(validator.ValidationErrors); ok {
			errors := make([]gin.H, 0, len(validationErrs))
			for _, fieldErr := range validationErrs {
				errors = append(errors, gin.H{
					"field": fieldErr.Namespace(),
					"tag":   fieldErr.Tag(),
				})
			}

			c.JSON(http.StatusBadRequest, gin.H{
				"message": "validation failed",
				"errors":  errors,
			})
			return false
		}

		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request payload"})
		return false
	}

	return true
}
