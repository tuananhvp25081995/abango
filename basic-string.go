package abango

import (
	"bytes"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	time "time"
	"unicode/utf8"
)

func okLog(s string) {
	log.Println("[OK] " + s)
}
func errLog(s string, err error) {
	log.Println("[ERROR] " + s + ": " + err.Error())
}

func prpbPair(keyLeng int) ([]byte, []byte) {

	prKey, _ := rsa.GenerateKey(rand.Reader, keyLeng)
	pbKey := &prKey.PublicKey

	prBytes := x509.MarshalPKCS1PrivateKey(prKey)
	prMem := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: prBytes,
	})

	// Bytes: 에 직접 넣으면 런타임에서 에러남.(중요!!)
	pbBytes, err := x509.MarshalPKIXPublicKey(pbKey)
	if err != nil {
		fmt.Println(err)
	}
	pbMem := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: pbBytes,
	})

	return prMem, pbMem
}

func mySignature(prKey []byte, msg []byte) ([]byte, error) { // msg 245=(256-11)bytes 이하

	block, _ := pem.Decode(prKey)
	if block == nil {
		// fmt.Println("Error: pem.Decode in mySignature")
		return nil, myErr("pem.Decode", nil)
	}
	priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, myErr("x509.ParsePKCS1PrivateKey", err)
	}

	sign, err := rsa.SignPKCS1v15(nil, priv, crypto.Hash(0), msg)
	if err != nil {
		return nil, myErr("rsa.SignPKCS1v15", err)
	}
	return sign, nil

}

func myOriginal(pubKey []byte, msg []byte) ([]byte, error) {

	block, _ := pem.Decode(pubKey)
	if block == nil {
		return nil, myErr("pem.Decode", nil)
	}
	pubInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, myErr("x509.ParsePKIXPublicKey", err)
	}
	pbKey := pubInterface.(*rsa.PublicKey)

	c := new(big.Int)
	m := new(big.Int)
	m.SetBytes(msg)
	e := big.NewInt(int64(pbKey.E))
	c.Exp(m, e, pbKey.N)
	out := c.Bytes()
	skip := 0
	for i := 2; i < len(out); i++ {
		if i+1 >= len(out) {
			break
		}
		if out[i] == 0xff && out[i+1] == 0 {
			skip = i + 2
			break
		}
	}
	return out[skip:], nil
}

func pbEncrypt(publicKey []byte, msg []byte) ([]byte, error) {
	origData := msg
	block, _ := pem.Decode(publicKey)

	if block == nil {
		return nil, myErr("pem.Decode", nil)
	}

	pubInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, myErr("x509.ParsePKIXPublicKey", err)
	}
	pub := pubInterface.(*rsa.PublicKey)

	label := []byte("")
	sha256hash := sha256.New()
	enBytes, err := rsa.EncryptOAEP(sha256hash, rand.Reader, pub, origData, label)
	if err != nil {
		return nil, myErr("rsa.EncryptOAEP", err)
	}

	return enBytes, nil
}

func prDecrypt(privateKey []byte, msg []byte) ([]byte, error) {

	ciphertext := msg
	block, _ := pem.Decode(privateKey)
	if block == nil {
		return nil, myErr("pem.Decode", nil)
	}
	priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, myErr("x509.ParsePKCS1PrivateKey", err)
	}

	label := []byte("")
	sha256hash := sha256.New()
	deBytes, err := rsa.DecryptOAEP(sha256hash, rand.Reader, priv, ciphertext, label)
	if err != nil {
		return nil, myErr("rsa.DecryptOAEP", err)
	}
	return deBytes, nil
}

func randString(i int) string {
	b := make([]byte, i)
	rand.Read(b)
	return (base64.URLEncoding.EncodeToString(b))[0:i]
}

// func randNumber(len int) string { // 나중에 코드 반드시 리팩토링 할 것
// 	a := make([]int, len)
// 	for i := 0; i <= len-1; i++ {
// 		a[i] = rand.Intn(len)
// 		return strings.Trim(strings.Replace(fmt.Sprint(a), " ", "", -1), "[]")
// 	}
// }

func randBytes(i int) []byte {
	return []byte(randString(i))
}

func addBase64Padding(value string) string {
	m := len(value) % 4
	if m != 0 {
		value += strings.Repeat("=", 4-m)
	}

	return value
}

func removeBase64Padding(value string) string {
	return strings.Replace(value, "=", "", -1)
}

func Pad(src []byte) []byte {
	padding := aes.BlockSize - len(src)%aes.BlockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(src, padtext...)
}

func Unpad(src []byte) ([]byte, error) {
	length := len(src)
	unpadding := int(src[length-1])

	if unpadding > length {
		return nil, errors.New("unpad error. This could happen when incorrect myEncryption key is used")
	}

	return src[:(length - unpadding)], nil
}

func myEncrypt(key []byte, text []byte) ([]byte, error) {

	block, err := aes.NewCipher(key)
	if err != nil {
		fmt.Println("Error: NewCipher in myEncrypt - " + err.Error())
		return nil, err
	}

	msg := Pad(text)
	ciphertext := make([]byte, aes.BlockSize+len(msg))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, myErr("io.ReadFull", err)
	}

	cfb := cipher.NewCFBEncrypter(block, iv)
	cfb.XORKeyStream(ciphertext[aes.BlockSize:], msg)
	finalMsg := removeBase64Padding(base64.URLEncoding.EncodeToString(ciphertext))

	return []byte(finalMsg), nil
}

func myDecrypt(key []byte, text []byte) ([]byte, error) {

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, myErr("aes.NewCipher", err)
	}
	decodedMsg, err := base64.URLEncoding.DecodeString(addBase64Padding(string(text)))
	if err != nil {
		return nil, myErr("base64.URLEncoding.DecodeString, Possibley Decryption string is too long", err)
	}

	if (len(decodedMsg) % aes.BlockSize) != 0 {
		return nil, myErr("aes.BlockSize-blocksize must be multipe of decoded message length", err)
	}

	iv := decodedMsg[:aes.BlockSize]
	msg := decodedMsg[aes.BlockSize:]

	cfb := cipher.NewCFBDecrypter(block, iv)
	cfb.XORKeyStream(msg, msg)

	unpadMsg, err := Unpad(msg)
	if err != nil {
		return nil, myErr("Unpad", err)
	}
	return unpadMsg, nil
}

func myHash(data []byte, leng int) []byte {
	hash := sha256.New() //SHA-3 규격임.
	hash.Write(data)

	mdStr := base64.URLEncoding.EncodeToString(hash.Sum(nil))

	rtn := ""
	if leng == 0 {
		rtn = mdStr
	} else {
		rtn = mdStr[10 : 10+leng]
	}
	return []byte(rtn)
}

func myToken(leng int) string {
	b := make([]byte, leng)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func bboxCombine(keyRaw []byte, a []byte, b []byte) ([]byte, error) {

	key := getCnt(keyRaw, 16)

	var e1 []byte
	e1 = append(e1, a...)
	e1 = append(e1, []byte("|||")...)
	e1 = append(e1, b...)

	e2, err := myEncrypt(key, e1)
	if err != nil {
		return nil, myErr("first myEncrypt", err)
	}
	// e2, err := myEncrypt([]byte(key), []byte(b))
	// if err != nil {
	// 	return nil, myErr("second myEncrypt", err))
	// }

	reverseKey := reverseBytes(key)
	e3, err := myEncrypt(reverseKey, e2)
	if err != nil {
		return nil, myErr("third myEncrypt", err)
	}

	return e3, nil
}

func bboxSplit(keyRaw []byte, c []byte) ([]byte, []byte, error) {
	//키값이 바뀐 경우 에러를 리턴하지 않고 runtime error 발생하므로 error 리턴 변수를 없앰.
	key := getCnt(keyRaw, 16)
	reverseKey := reverseBytes(key)

	d1, err := myDecrypt([]byte(reverseKey), []byte(c))
	if err != nil {
		return nil, nil, myErr("first myDecrypt", err)
	}
	d2, err := myDecrypt([]byte(key), d1)
	if err != nil {
		return nil, nil, myErr("second myDecrypt", err)
	}
	// d3, err := myDecrypt([]byte(key), []byte(result[1]))
	// if err != nil {
	// 	myErr("second myDecrypt", err)
	// 	return nil, nil
	// }

	result := strings.Split(string(d2), "|||")
	return []byte(result[0]), []byte(result[1]), nil
}
func bboxCombine_original(keyRaw []byte, a []byte, b []byte) ([]byte, error) {

	key := getCnt(keyRaw, 16)
	e1, err := myEncrypt(key, a)
	if err != nil {
		return nil, myErr("first myEncrypt", err)
	}
	e2, err := myEncrypt([]byte(key), []byte(b))
	if err != nil {
		return nil, myErr("second myEncrypt", err)
	}
	var e3 []byte
	e3 = append(e3, e1...)
	e3 = append(e3, []byte("|||")...)
	e3 = append(e3, e2...)

	reverseKey := reverseBytes(key)
	e4, err := myEncrypt(reverseKey, e3)
	if err != nil {
		return nil, myErr("third myEncrypt", err)
	}

	return e4, nil
}

func bboxSplit_original(keyRaw []byte, c []byte) ([]byte, []byte) {
	//키값이 바뀐 경우 에러를 리턴하지 않고 runtime error 발생하므로 error 리턴 변수를 없앰.
	key := getCnt(keyRaw, 16)
	reverseKey := reverseBytes(key)

	d1, err := myDecrypt([]byte(reverseKey), []byte(c))
	result := strings.Split(string(d1), "|||")

	d2, err := myDecrypt([]byte(key), []byte(result[0]))
	if err != nil {
		myErr("first myDecrypt", err)
		return nil, nil
	}
	d3, err := myDecrypt([]byte(key), []byte(result[1]))
	if err != nil {
		myErr("second myDecrypt", err)
		return nil, nil
	}

	return d2, d3
}

func reverseString(s string) string {
	cs := make([]rune, utf8.RuneCountInString(s))
	i := len(cs)
	for _, c := range s {
		i--
		cs[i] = c
	}
	return string(cs)
}

func reverseBytes(s []byte) []byte {
	cs := make([]byte, len(s))
	i := len(cs)
	for _, c := range s {
		i--
		cs[i] = c
	}
	return cs
}

func getCnt(s []byte, cnt int) []byte {
	// ret := ""
	var ret []byte
	if len(s) > cnt {
		ret = s[0:cnt]
	} else if len(s) < cnt {
		ret = append(s, strings.Repeat("=", cnt-len(s))...)
	} else {
		ret = s
	}
	return ret
}

func agErr(s string, e error, amsg *string) string {
	fmt.Println("== agErr ", strings.Repeat("=", 90))
	// fpcs := make([]uintptr, 1)
	// n := runtime.Callers(2, fpcs)
	// if n == 0 {
	// 	fmt.Println("MSG: NO CALLER")
	// }
	// // caller := runtime.FuncForPC(fpcs[0] - 1)
	// caller := runtime.FuncForPC(fpcs[0])
	// // fmt.Println(caller.FileLine(fpcs[0] - 1))
	// fmt.Println(caller.FileLine(fpcs[0]))
	// fmt.Println(caller.Name())
	emsg := ""
	if e != nil {
		emsg = "Error: " + e.Error() + " in " + s
	} else {
		emsg = "Error: error is nil" + " in " + s // e 가 nil 인 상태에서 Error() 인용시 runtime error
	}
	fmt.Println(emsg, "\n")
	whereami(2)
	whereami(3)
	fmt.Println(strings.Repeat("=", 100))
	return emsg
}

func myErr(s string, e error) error {
	fmt.Println("== myErr ", strings.Repeat("=", 90))
	emsg := ""
	if e != nil {
		emsg = "Error: " + e.Error() + " in " + s
	} else {
		emsg = "Error: error is nil" + " in " + s
	}
	fmt.Println(emsg, "\n")
	whereami(2)
	whereami(3)
	whereami(4)
	fmt.Println(strings.Repeat("=", 100))
	return errors.New(emsg)
}

func tp(a ...interface{}) {
	fmt.Println(a)
}

func getNow() time.Time {
	loc, _ := time.LoadLocation("UTC")
	return time.Now().In(loc)
}

func whereami(i int) {
	function, file, line, _ := runtime.Caller(i)
	fmt.Printf("  %d.File: %s - %d  %s\n   func: %s \n", i, chopPath(file), line, file, runtime.FuncForPC(function).Name())
}

func WhereAmI(depthList ...int) {
	var depth int
	if depthList == nil {
		depth = 1
	} else {
		depth = depthList[0]
	}
	// function, file, line, _ := runtime.Caller(depth)

	for i := 0; i < depth+1; i++ {

		function, file, line, _ := runtime.Caller(i)
		fmt.Printf("==Level %d==\n", i)
		fmt.Printf("File: %s - %d  %s\nFunction: %s \n", chopPath(file), line, file, runtime.FuncForPC(function).Name())
	}
	fmt.Printf("==End==\n")

	return
}

// return the source filename after the last slash
func chopPath(original string) string {
	i := strings.LastIndex(original, "/")
	if i == -1 {
		return original
	} else {
		return original[i+1:]
	}
}

func getESignature(omfa []byte, upr []byte, spb_d []byte, amsg *string) string {

	preSign, err9 := mySignature(upr, omfa)
	if err9 != nil {
		agErr("preSign", err9, amsg)
		return ""
	}

	e_sign1, err1 := pbEncrypt(spb_d, preSign[:100])
	if err1 != nil {
		agErr("preSign", err1, amsg)
		return ""
	}

	e_sign2, err2 := pbEncrypt(spb_d, preSign[100:])
	if err2 != nil {
		agErr("preSign", err2, amsg)
		return ""
	}

	e_sign := append(e_sign1, []byte("|||")...)
	e_sign = append(e_sign, e_sign2...)

	return base64.URLEncoding.EncodeToString(e_sign)
}

func getOriginal(spr []byte, upb []byte, e_signature string) string {

	tmp_sign, _ := base64.URLEncoding.DecodeString(e_signature)
	e_sign := strings.Split(string(tmp_sign), "|||")

	preSign1, _ := prDecrypt(spr, []byte(e_sign[0]))
	preSign2, _ := prDecrypt(spr, []byte(e_sign[1]))
	preSign := append(preSign1, preSign2...)
	mfa, _ := myOriginal(upb, preSign)

	return string(mfa)
}

func dummySignature(salt []byte) ([]byte, error) {

	slen := len(salt) - 1
	interval, err := strconv.Atoi(string(salt[slen:]))

	if err != nil {
		return nil, myErr("Wrong DSALT format", err)
	}

	ran := randBytes(interval * slen)
	for i := 0; i < slen; i++ {
		ran[i*interval] = salt[i]
	}
	return ran, nil

}

func dummyVeryfy(salt []byte, sign []byte) (bool, error) {

	slen := len(salt) - 1
	interval, err := strconv.Atoi(string(salt[slen]))

	if err != nil {
		return false, myErr("Wrong DSALT format", err)
	}

	esalt := make([]byte, len(salt))
	for i := 0; i < slen; i++ {
		esalt[i] = sign[i*interval]
	}

	esalt[slen] = salt[slen]
	return true, nil
}

func structToMap(in interface{}, tag string) (map[string]interface{}, error) {
	out := make(map[string]interface{})

	v := reflect.ValueOf(in)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	// we only accept structs
	if v.Kind() != reflect.Struct {
		fmt.Errorf("ToMap only accepts structs; got %T", v)
		return nil, myErr("only accepts structs", nil)
	}

	typ := v.Type()
	for i := 0; i < v.NumField(); i++ {
		// gets us a StructField
		fi := typ.Field(i)
		if tagv := fi.Tag.Get(tag); tagv != "" {
			out[tagv] = v.Field(i).Interface()
		}
	}
	return out, nil
}

func fileCopy(src, dst string) error { // Copy시메모리 소모 없슴.
	sFile, err := os.Open(src)
	if err != nil {
		return myErr("File Open", err)
	}
	defer sFile.Close()

	eFile, err := os.Create(dst)
	if err != nil {
		return myErr("File Create", err)
	}
	defer eFile.Close()

	_, err = io.Copy(eFile, sFile) // first var shows number of bytes
	if err != nil {
		return myErr("File Copy", err)
	}

	err = eFile.Sync()
	if err != nil {
		return myErr("File Open", err)
	}
	return nil
}

func parentDir() string { // Copy시메모리 소모 없슴.
	workDir, _ := os.Getwd()
	sp := strings.Split(workDir, "/")
	parentDir := ""
	for i := 1; i < len(sp)-1; i++ {
		parentDir += "/" + sp[i]
	}
	return parentDir
}

func getOTP(n int) []byte {
	const letters = "0123456789"
	bytes := make([]byte, n)
	_, err := rand.Read(bytes)
	if err != nil {
		myErr("rand.Read", err)
	}

	for i, b := range bytes {
		bytes[i] = letters[b%byte(len(letters))]
	}
	return bytes
}

func MkDir(dir string, perm os.FileMode) (int, error) {

	dirSw := 0
	if _, err := os.Stat(dir); err != nil {

		err := os.MkdirAll(dir, perm)
		if err == nil {
			fmt.Println(dir, " directory was created !")
			dirSw = 1
		} else {
			return dirSw, myErr("CAN NOT create directory :"+dir, err)
		}
	}

	return dirSw, nil
}

func myEncr256(key []byte, nonce []byte, plaintext []byte) ([]byte, error) {
	// The key argument should be the AES key, either 16 or 32 bytes
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, myErr("NewCipher", err)
	}

	// Never use more than 2^32 random nonces with a given key because of the risk of a repeat.
	// nonce := make([]byte, 12) // Do not change 12
	// if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
	// 	return nil,  myErr("io.ReadFull", err)
	// }

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, myErr("cipher.NewGCM", err)
	}

	text := aesgcm.Seal(nil, nonce, plaintext, nil)
	return text, nil
}

func myDecr256(key []byte, nonce []byte, text []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, myErr("NewCipher", err)
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, myErr("cipher.NewGCM", err)
	}

	plaintext, err := aesgcm.Open(nil, nonce, text, nil)
	if err != nil {
		return nil, myErr("aesgcm.Open", err)
	}
	return plaintext, nil
}

func getNowUnix(sec ...int) int64 {
	var ret int64
	if sec == nil {
		ret = time.Now().UTC().Unix()
	} else {
		ret = time.Now().Add(time.Duration(sec[0]) * time.Second).UTC().Unix()
	}
	return ret
}
