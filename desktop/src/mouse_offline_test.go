package main
import (
"bytes"
"crypto/sha256"
"os"
"path/filepath"
"strings"
"testing"
)
func TestSenseVoiceManifestPinning(t *testing.T){
 if len(senseVoiceAssets)!=2{t.Fatal("missing pinned model assets")}
 for _,asset:=range senseVoiceAssets{
  if len(asset.SHA)!=64||asset.Size<100000||!strings.HasPrefix(asset.URL,"https://huggingface.co/"){
   t.Fatalf("unpinned asset: %s",asset.Name)
  }
 }
}
func TestSenseVoiceRejectsTruncatedCache(t *testing.T){
 dir:=t.TempDir()
 a:=senseVoiceAsset{Name:"mock.onnx",Size:3}
 hash:=sha256.Sum256([]byte("abc"))
 a.SHA=fmtHash(hash[:])
 path:=filepath.Join(dir,a.Name)
 if err:=os.WriteFile(path,[]byte("ab"),0600);err!=nil{t.Fatal(err)}
 if modelHashMatches(path,a){t.Fatal("truncated file accepted")}
 if err:=os.WriteFile(path,[]byte("abc"),0600);err!=nil{t.Fatal(err)}
 if !modelHashMatches(path,a){t.Fatal("valid file rejected")}
}
func fmtHash(b []byte) string{
 const digits="0123456789abcdef"
 out:=make([]byte,len(b)*2)
 for i,v:=range b{out[i*2]=digits[v>>4];out[i*2+1]=digits[v&15]}
 return string(out)
}
func TestVerifiedModelCopyLimit(t *testing.T){
 var dst bytes.Buffer
 h:=sha256.New()
 copied,err:=copyVerifiedModel(&dst,h,strings.NewReader("model"),5,nil)
 if err!=nil||copied!=5||dst.String()!="model"{t.Fatalf("copy failed: %v %d",err,copied)}
}
