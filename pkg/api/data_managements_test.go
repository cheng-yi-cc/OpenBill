package api

import (
	"testing"

	"github.com/mayswind/ezbookkeeping/pkg/models"
	"github.com/mayswind/ezbookkeeping/pkg/services"
	"github.com/mayswind/ezbookkeeping/pkg/utils"
	"github.com/stretchr/testify/require"
)

func TestVerifyClearPassword(t *testing.T) {
	a := &DataManagementsApi{users: services.Users}
	passwordless := &models.User{Uid: 1, Salt: "salt123456", Password: ""}
	require.True(t, a.verifyClearPassword("", passwordless))
	require.False(t, a.verifyClearPassword("123456", passwordless))

	protected := &models.User{Uid: 2, Salt: "salt123456", Password: utils.EncodePassword("secret123", "salt123456")}
	require.True(t, a.verifyClearPassword("secret123", protected))
	require.False(t, a.verifyClearPassword("", protected))
	require.False(t, a.verifyClearPassword("wrongpass", protected))
}
