package service

import (
	"args"
	"crypto/md5"
	"encoding/hex"
	"entity"
	"err"
	"math"
	"math/rand"
	"middlewares"
	"model"
	"response"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/kataras/iris"
)

// UserFormData .
type UserFormData struct {
	Email    string `form:"email"`
	Password string `form:"password"`
}

// ResetFormData .
type ResetFormData struct {
	Email       string `form:"email"`
	AuthCode    string `form:"authCode"`
	Newpassword string `form:"newPassword"`
}

func encodePassword(initPassword string) (password string) {
	md5Hash := md5.New()
	md5Hash.Write([]byte(initPassword))
	password = hex.EncodeToString(md5Hash.Sum(nil))
	return
}

// UserLogin .
func UserLogin(ctx iris.Context) {
	var user entity.User
	var has bool
	userForm := UserFormData{}

	er := ctx.ReadForm(&userForm)
	err.CheckErrWithPanic(er)

	email := userForm.Email
	password := encodePassword(userForm.Password)

	user, has, er = model.GetUserByEmailAndPassword(email, password)
	err.CheckErrWithPanic(er)
	if !has {
		response.Forbidden(ctx, iris.Map{})
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":  user.ID,
		"exp": jwt.StandardClaims{ExpiresAt: time.Now().Add(time.Hour * 24).Unix()},
	})
	t, er := token.SignedString([]byte(args.SecretKey))
	err.CheckErrWithPanic(er)

	ctx.ResponseWriter().Header().Set("Authorization", "Bearer "+t)
	response.OK(ctx, iris.Map{
		"userid": user.ID,
	})
}

// UserLogout .
func UserLogout(ctx iris.Context) {
	delete(ctx.ResponseWriter().Header(), "Authorization")
	response.OK(ctx, iris.Map{})
}

// UserRegister .
func UserRegister(ctx iris.Context) {
	userForm := UserFormData{}

	er := ctx.ReadForm(&userForm)
	err.CheckErrWithPanic(er)

	email := userForm.Email
	password := encodePassword(userForm.Password)

	has, er := model.CheckUserByEmail(email)
	err.CheckErrWithPanic(er)
	if has {
		response.Conflict(ctx, iris.Map{})
		return
	}

	user, er := model.NewUser(email, password)
	err.CheckErrWithPanic(er)

	response.OK(ctx, iris.Map{
		"userid": user.ID,
	})
}

// ChangePassword ...
// route: [/users/change_password] [PUT]
// pre: the user is in the session
// post: the password has been updated
func ChangePassword(ctx iris.Context) {
	userID := middlewares.GetUserID(ctx)
	oldPassword := encodePassword(ctx.FormValue("oldPassword"))
	newPassword := encodePassword(ctx.FormValue("newPassword"))

	_, has, er := model.GetUserByIDAndPassword(userID, oldPassword)
	if er != nil || !has {
		response.Forbidden(ctx, iris.Map{})
		return
	}

	er = model.ChangePassword(userID, newPassword)
	err.CheckErrWithPanic(er)

	response.OK(ctx, iris.Map{})
}

// gen auth code for reset password
// route: [/users/reset_password] [PUT]
// pre: None
// post: store the map info of auth code of the user
func genAuthCode(ctx iris.Context) {
	email := ctx.FormValue("email")
	user, has, er := model.GetUserByEmail(email)
	err.CheckErrWithPanic(er)

	if has == false {
		response.Forbidden(ctx, iris.Map{})
		return
	}

	has, er = model.CheckAuthCodeByUser(user.ID)
	err.CheckErrWithPanic(er)

	newCode := genRandAuthCode(args.AuthCodeSize)
	// if exists, update, or, insert
	if has == false {
		_, er := model.NewAuthCode(user.ID, newCode)
		err.CheckErrWithPanic(er)
	} else {
		er = model.UpdateAuthCode(user.ID, newCode)
		err.CheckErrWithPanic(er)
	}

	response.OK(ctx, iris.Map{
		"authCode": newCode,
	})
}

// ResetPassword ...
// route : [/users/reset_password] [PUT]
// pre: there are 3 key in the request form: "email", "authCode", "newPassword"
// post: if authCode is valid with the user, the password will have been reset
func ResetPassword(ctx iris.Context) {
	info := ResetFormData{}
	er := ctx.ReadForm(&info)
	err.CheckErrWithPanic(er)

	// check whether the email is valid
	user, has, er := model.GetUserByEmail(info.Email)
	err.CheckErrWithPanic(er)
	if has == false {
		response.Forbidden(ctx, iris.Map{})
		return
	}

	// check whether the code is stored in the database
	authCode, has, er := model.GetAuthCodeByUserAndCode(user.ID, info.AuthCode)
	err.CheckErrWithPanic(er)
	if has == false {
		response.Forbidden(ctx, iris.Map{})
		return
	}

	yes, er := isBefore(args.AuthCodeLifeTime, authCode.UpdateTime)
	err.CheckErrWithPanic(er)

	// now - AuthCodeLifeTime(minutes) is not before codeUpdateTime
	if yes == false {
		response.Forbidden(ctx, iris.Map{})
		return
	}

	er = model.ChangePassword(user.ID, info.Newpassword)
	err.CheckErrWithPanic(er)

	response.OK(ctx, iris.Map{})
}

// gen numeric auth code with size bits
func genRandAuthCode(size int) string {
	maxOne := int32(math.Pow10(size))
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	return string(rnd.Int31n(maxOne))
}

// now() - lefiTime(minutes) is before pastTime
func isBefore(lifeTime int, pastTime time.Time) (bool, error) {
	now := time.Now()
	m, er := time.ParseDuration("-" + string(lifeTime) + "m")

	// AuthCodeLifeTime minutes ago
	cmpTime := now.Add(m)
	return cmpTime.Before(pastTime), er
}
