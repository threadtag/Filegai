// Author: Hengyi Jiang <hengyi.jiang@gmail.com>
// 2022-04-16 
// Version 0.1
/*
BSD 2-Clause License

Copyright (c) 2022, Hengyi Jiang
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice, this
   list of conditions and the following disclaimer.

2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/

package main
import(
    "fmt"
    "database/sql"
    _ "github.com/mattn/go-sqlite3"
    "io/ioutil"
    "os"
    "os/exec"
    "syscall"
    "strings"
    "strconv"
    "errors"
    "github.com/gin-gonic/gin"
    "net/http"
    "bytes"
    "math/rand"
    "time"
    "regexp"
    "html/template"
    "path/filepath"
    "flag"
)

type Fnode struct{
    Name string
    IsDir bool
    Dev int32  // type is the same as the API return value
    Ino uint64  // type is the same as the API return value
    Parent_dev int32
    Parent_ino uint64
}

type Fnode_view struct{
    Name string
    IsDir bool
    Dev int32  // type is the same as the API return value
    Ino uint64  // type is the same as the API return value
    Parent_dev int32
    Parent_ino uint64
    Tag string
    Note string
    Color string
    Note_visible string 
    Active_css_class string 
    Pin_class string
    Pin_value string 
}


type Db_column struct{
    Type rune // char , varchar, text ==> s, bool =>b, int ==>i, datetim=>t
    Name string
    Value string
}

type Db_table struct{
    Name string
    Columns map[string]bool
    Data map[string]string  // for insert and update
}

type Note_record struct{
    Tag string
    Name string
    Note string
    Color int
    Color_str string
    File_dir string
    File_name string
    Ndate string
}

type  Resource_record struct{
    Tag string
    Name string
    Rs_type int
    Page int
    Rs_date string
    Ref_count int
}


func (tab *Db_table) set_name(name string)  *Db_table{
    tab.Name = name
    tab.Columns = make(map[string]bool)
    tab.Data = make(map[string]string)
    return tab
}

func (tab *Db_table) add_column(name string, to_quote bool)  *Db_table{
    tab.Columns[name]=to_quote
    return tab
}

func (tab *Db_table) set(name string, value string) *Db_table{
    to_quote,ok := tab.Columns[name]
    if ok{
        if to_quote{
            tab.Data[name]="\""+strings.ReplaceAll(value,"\"","\"\"")+"\""
        }else{
            tab.Data[name]=value
        }
    }
    return tab
}

func (tab *Db_table) clear(){
    tab.Name = ""
    tab.Columns=make(map[string]bool)
    tab.Data=make(map[string]string)
}

func (tab *Db_table) pack_insert() string{
    var cols []string
    var values []string
    for col,value :=range tab.Data{
        cols=append(cols,col)
        values =append(values,value)        
    }
    str :="INSERT INTO "+tab.Name+"("+strings.Join(cols,",")+") VALUES ("+ strings.Join(values,",") +")"
    return str
}

func (tab *Db_table) pack_select(q_cols string, order string, limit string) string{
    where_str :=""
    where_arr:=[]string{}
    order_str :=""
    limit_str :=""
    for col,value := range tab.Data{
        where_arr = append(where_arr,col+"="+value)
    }
    if len(where_arr)>0{
        where_str = " WHERE "+strings.Join(where_arr," and ")
    }
    if order !=""{
        order_str = " ORDER BY "+order
    }
    if limit !=""{
        limit_str =" LIMIT "+limit
    }
    str :="SELECT "+q_cols+" FROM  "+tab.Name+" "+where_str + order_str + limit_str
    return str
}

func (tab *Db_table) pack_count(count_col string) string{
    return tab.pack_select("count(*) as "+count_col,"","")
}


func (tab *Db_table) pack_update(check []string) string{
    set_arr:=[]string{}
    where_str :=""
    where_arr:=[]string{}
    for col,value :=range tab.Data{
        set_arr=append(set_arr,col+"="+value)
    }

    for _,col := range check{
        where_arr = append(where_arr,col+"="+tab.Data[col])
    }
    if len(where_arr)>0{
        where_str = " WHERE "+strings.Join(where_arr," and ")
    }
    if len(set_arr)>0{
        str :="UPDATE "+tab.Name+" SET "+strings.Join(set_arr,",")+where_str
        return str
    }else{
        return ""
    }    
}

func (tab *Db_table) pack_delete() string{
    where_str :=""
    where_arr:=[]string{}
    for col,value := range tab.Data{
        where_arr = append(where_arr,col+"="+value)
    }
    if len(where_arr)>0{
        where_str = " WHERE "+strings.Join(where_arr," and ")
    }
    return "DELETE FROM "+tab.Name+where_str
}


// for fs handling
func folder_entries(path string) []*Fnode{
    var values  []*Fnode
    pnt_fileinfo, _ := os.Stat(path)
    pnt_stat, ok := pnt_fileinfo.Sys().(*syscall.Stat_t)
    if !ok {
        return values //empty
    }
    fileInfos,err := ioutil.ReadDir(path)
    if err !=nil{return values}
    for _,info :=range fileInfos{
        stat, ok := info.Sys().(*syscall.Stat_t)
        if ok{
            if ! strings.HasPrefix(info.Name(),"."){
                values=append(values,&Fnode{info.Name(),info.IsDir(),stat.Dev,stat.Ino,pnt_stat.Dev,pnt_stat.Ino})
            }
        }        
    }
    return values
}

func get_Fnode(path string,is_root bool) (*Fnode,error){
    var result Fnode
    delim :=sys_delim()
    if delim=="\\"{
        path = strings.ReplaceAll(path, "/","\\")
    }
    info, err := os.Stat(path)
    if err!=nil{
        return &result,err
    }
    stat, ok := info.Sys().(*syscall.Stat_t)
    if !ok {
        return &result,errors.New("error in geting stat") //empty
    }
    
    result.IsDir=info.IsDir()
    result.Dev=stat.Dev
    result.Ino=stat.Ino
    if is_root{
        ensure_folder(&path,delim)
        if strings.HasSuffix(path,delim){
            result.Name=path[0:(len(path)-1)]
        }else{
            result.Name=path
        }        
        result.Parent_dev=stat.Dev
        result.Parent_ino=stat.Ino
    }else{
        result.Name=info.Name()
        var tmp_path string
        if strings.HasSuffix(path,delim){
            tmp_path =path_dir_name(path[0:(len(path)-1)],delim)
        }else{
            tmp_path =path_dir_name(path[0:(len(path))],delim)
        }
        info, _ := os.Stat(tmp_path)
        stat, _ := info.Sys().(*syscall.Stat_t)
        result.Parent_dev=stat.Dev
        result.Parent_ino=stat.Ino
    }
    return &result,nil
}

func (fnode *Fnode) dev_ino() string{
    return strconv.FormatUint(uint64(fnode.Dev),10)+"_"+strconv.FormatUint(fnode.Ino,10)
}

func (fnode *Fnode) parent_dev_ino()string{
    return strconv.FormatUint(uint64(fnode.Parent_dev),10)+"_"+strconv.FormatUint(fnode.Parent_ino,10)
}

func (fnode *Fnode) device_id() string{
    return strconv.FormatUint(uint64(fnode.Dev),10)
}

func (fnode *Fnode) ino() string{
    return strconv.FormatUint(fnode.Ino,10)
}

func (fnode *Fnode) parent_ino() string{
    return strconv.FormatUint(fnode.Parent_ino,10)
}



// extending the db object methods
func do_insert(db_link *sql.DB,sql_insert string)(int64,error){
    sql_run, err := db_link.Prepare(sql_insert)
    if err !=nil{
        fmt.Println("?? error in db operation:")
        fmt.Println(sql_insert)
        return 0,err
    }
    res, err :=sql_run.Exec()
    if err !=nil{
        return 0,err
    }
    id, err := res.LastInsertId()
    if err !=nil{
        return 0,err
    }
    return id,err
}

func do_exec(db_link *sql.DB,sql_cmd string)(int64,error){
    sql_run, err := db_link.Prepare(sql_cmd)
    if err !=nil{
        fmt.Println("?? error in db operation:")
        fmt.Println(sql_cmd)
        return 0,err
    }
    res, err :=sql_run.Exec()
    if err !=nil{
        return 0,err
    }
    count, err := res.RowsAffected()
    if err !=nil{
        return 0,err
    }
    return count,nil
}
func do_update(db_link *sql.DB,sql_update string)(int64,error){
    return do_exec(db_link,sql_update)
}

func do_delete(db_link *sql.DB,sql_delete string)(int64,error){
    return do_exec(db_link,sql_delete)
}

func do_count(db_link *sql.DB,sql_count string)(int64,error){
    rows, err := db_link.Query(sql_count)
    if err !=nil{
        return 0,err
    }
    rows.Next()
    var count  int64
    err = rows.Scan(&count)
    rows.Close()
    if err !=nil{
        return 0,err
    }
    return count,nil
}

func do_select_id(db_link *sql.DB,sql_select string)([]int64,error){
    rows, err := db_link.Query(sql_select)
    r :=[]int64{}
    if err !=nil{
        return r,err
    }
    var temp int64
    for rows.Next(){        
        err = rows.Scan(&temp)
        if err ==nil{
            r=append(r,temp)
        }
    }
    rows.Close()
    return r,nil
}


//====================================================================================================
// Ino_tree

func register_ino(db_link *sql.DB,node *Fnode)(bool,error){
// do query first
    table:=new(Db_table)
    table.set_name("ino_tree").add_column("host_name",true)
    table.add_column("device_id",false).add_column("ino",false).add_column("parent_ino",false)
    table.add_column("name",true).add_column("type",true).add_column("state",true)
    
    tp := "f"
    if node.IsDir {
        tp="d"
    }
    host_name:=get_host_name()
    table.set("host_name",host_name)
    table.set("device_id",node.device_id()).set("ino",node.ino()) 
    count,err := do_count(db_link,table.pack_count("cnt"))
    if err !=nil{
        return false,err
    }
    if count >1{
        // delete all obsoleted nodes
        _,err=do_delete(db_link,table.pack_delete())
        if err !=nil{
            return false, err
        }
    }

    if count ==1 {
        // update the nodes
        table.set("name",node.Name).set("parent_ino",strconv.FormatUint(node.Parent_ino,10)).set("type",tp).set("state","a")
        _,err :=update_ino(db_link,node)
        if err !=nil{
            return false,err
        }
    }

    if count==0 || count>1{
        table.set("name",node.Name).set("parent_ino",strconv.FormatUint(node.Parent_ino,10)).set("type",tp).set("state","a")
        _,err = do_insert(db_link,table.pack_insert())
        if err !=nil{
            return false,err
        }
    }
    return true,nil
}


func update_ino(db_link *sql.DB,node *Fnode)(bool,error){
    table:=new(Db_table)
    table.set_name("ino_tree")
    table.add_column("device_id",false).add_column("ino",false).add_column("parent_ino",false)
    table.add_column("name",true).add_column("type",true).add_column("state",true)
    tp := "f"
    if node.IsDir {
        tp="d"
    }
    table.set("device_id",strconv.FormatUint(uint64(node.Dev),10)).set("ino",strconv.FormatUint(node.Ino,10))
    table.set("name",node.Name).set("parent_ino",strconv.FormatUint(node.Parent_ino,10)).set("type",tp).set("state","a")

    check := []string{"device_id","ino"}
    _,err:=do_update(db_link,table.pack_update(check))
    if err !=nil{
        return false,err
    }
    return true,nil
}

func delete_ino(db_link *sql.DB,node *Fnode)(bool,error){
    table:=new(Db_table)
    host_name :=get_host_name()
    table.set_name("ino_tree")
    table.add_column("device_id",false).add_column("ino",false).add_column("parent_ino",false)
    table.add_column("name",true).add_column("type",true).add_column("state",true).add_column("host_name",true)
    table.set("device_id",strconv.FormatUint(uint64(node.Dev),10)).set("ino",strconv.FormatUint(node.Ino,10)).set("host_name",host_name)
    _,err:=do_delete(db_link,table.pack_delete())
    if err !=nil{
        return false,err
    }
    return true,nil
}

func clear_ino(db_link *sql.DB,root_dir string)(bool,error){
    host_name :=get_host_name()
    fnode,err :=get_Fnode(root_dir,true)
    if err !=nil{
        return false,err
    }
    table:=new(Db_table)
    table.set_name("ino_tree")
    table.add_column("device_id",false).add_column("ino",false).add_column("parent_ino",false).add_column("host_name",true)
    table.add_column("name",true).add_column("type",true).add_column("state",true)
    table.set("device_id",strconv.FormatUint(uint64(fnode.Dev),10)).set("host_name",host_name)
    _,err =do_delete(db_link,table.pack_delete())
    if err !=nil{
        return false,err
    }
    return true,nil
}

func query_fnode(db_link *sql.DB,device_id uint64, ino uint64) (*Fnode,error){
    table:=new(Db_table)
    table.set_name("ino_tree")
    table.add_column("device_id",false).add_column("ino",false).add_column("parent_ino",false).add_column("host_name",true)
    table.add_column("name",true).add_column("type",true).add_column("state",true)
    host_name:=get_host_name()
    table.set("device_id",strconv.FormatUint(device_id,10)).set("ino",strconv.FormatUint(ino,10)).set("host_name",host_name)
    var node Fnode
    tp :="f"
    rows, err := db_link.Query(table.pack_select("device_id,ino,parent_ino,name,type","name asc","1"))
    if err !=nil{
        return &node,err
    }
    i :=0
    for rows.Next(){        
        err = rows.Scan(&node.Dev,&node.Ino,&node.Parent_ino,&node.Name,&tp)
        if err ==nil{
            node.IsDir=false
            if tp=="d"{
                node.IsDir=true
            }
        }
        i+=1
    }
    rows.Close()
    if i==0{
        return &node,errors.New("no result")
    }
    return &node,nil
}

func file_url(db_link *sql.DB,device_id uint64, ino uint64,max_level int,delim string) (string,error){
    node, err := query_fnode(db_link,device_id,ino)
    if max_level<0{
        // guard against infinite loop error
        return "",errors.New("0")
    }
    if err !=nil{
        return "",errors.New("1")
    }
    if node.Ino==node.Parent_ino{
        return node.Name+delim,nil
    }
    s:=""
    if node.IsDir{
        s=delim
    }
    parent,err:=file_url(db_link,device_id,node.Parent_ino,max_level-1,delim)
    if err !=nil{
        return "",errors.New("2")
    }
    // fmt.Printf("%s=>%s",node.Name,s)
    return parent+node.Name+s,nil
}

// set calculation
func inos_in_parent(db_link *sql.DB,device_id uint64,parent_id uint64)([]int64){
    table:=new(Db_table)
    host_name:=get_host_name()
    table.set_name("ino_tree")
    table.add_column("device_id",false).add_column("ino",false).add_column("parent_ino",false).add_column("host_name",true)
    table.add_column("name",true).add_column("type",true).add_column("state",true)
    table.set("parent_ino",strconv.FormatUint(parent_id,10)).set("device_id",strconv.FormatUint(device_id,10))
    table.set("host_name",host_name)
    ids,err:=do_select_id(db_link,table.pack_select("ino","",""))
    if err!=nil{
        var r []int64
        return r
    }
    return ids
}

func inos_to_update(nodes_fs []*Fnode,inos_db []int64) (map[uint64]bool){
    in_db := make(map[uint64]bool)
    result :=make(map[uint64]bool)
    for _,t := range(inos_db){
        in_db[uint64(t)]=true
    }
    for _, v :=range(nodes_fs){
        _, ok:=in_db[v.Ino]
        if ok{
            result[v.Ino]=true
        }
    }
    return result
}

// func inos_to_insert(nodes_fs []Fnode,inos_update map(uint64)bool)(map(uint64)bool){
//     // not used in the code
//     to_update := make(map[uint64]bool)
//     var result []uint64
//     for _,t := range(inos_update){
//         to_update[t]=true
//     }
//     for _, v :=range(nodes_fs){
//         _, ok:=to_update[v.Ino]
//         if !ok{
//             result = append(result,v.Ino)
//         }
//     }
//     return result
// }
func inos_to_delete(nodes_fs []*Fnode,inos_db []int64)(map[uint64]bool){
    to_update :=  inos_to_update(nodes_fs,inos_db)
    result := make(map[uint64]bool)
    for _, v :=range(inos_db){
        _, ok:=to_update[uint64(v)]
        if !ok{
            result[uint64(v)]=true
        }
    }
    return result
}



func refresh_folder(db_link *sql.DB,folder string,is_root bool){
    
    this_fnode,err := get_Fnode(folder,is_root)
    if err !=nil{
        return
    }
    device_id :=this_fnode.Dev
    if is_root{  
        register_ino(db_link,this_fnode)
    }
    folder_entries :=  folder_entries(folder)
    if is_root{
        folder_entries=append(folder_entries,this_fnode)
    }
    inos_db := inos_in_parent(db_link,uint64(this_fnode.Dev),this_fnode.Ino)
    delete_set := inos_to_delete(folder_entries,inos_db)

    var temp_fnode Fnode
    for _,ino := range(inos_db){
        _,ok := delete_set[uint64(ino)]
        if ok{
            // to delete
            // fmt.Printf("deleting ino:%s\n",strconv.FormatInt(ino,10))
            temp_fnode.Dev=device_id
            temp_fnode.Ino=uint64(ino)
            delete_ino(db_link,&temp_fnode)
        }
    }

    for _,node :=range(folder_entries){
        _,ok:=delete_set[node.Ino]
        if !ok{
            // to insert or update
            _,err=register_ino(db_link,node)
            if err !=nil{
                fmt.Printf("?? registering error:%q,%s,%d",err,node.Name,node.Ino)
            }
        }        
    }
}

//====================================================================================================
// common functions

func file_exists(path string) (bool, error) {
    _, err := os.Stat(path)
    if err == nil { return true, nil }
    if os.IsNotExist(err) { return false, nil }
    return true, err
}

func get_abs_path(rel_path string) string {
    abs_path, err := filepath.Abs(rel_path)
    if err != nil {
        return rel_path
    }
    return abs_path
}

func folder_split(folder string,root_dir string,delim string) []string{
    var s []string
    var r []string
    var u string
    var tail string
    if !strings.HasSuffix(root_dir,delim){
        root_dir = root_dir+delim
    }

    if !strings.Contains(folder,root_dir){
        tail = folder
        u=""
    }else{
        r = append(r,root_dir)
        idx := strings.Index(folder,root_dir)
        tail = folder[(idx+len(root_dir)):]
        u=root_dir
    }
    s = strings.SplitAfter(tail,delim)
    
    for _,t := range(s){
        u=u+t
        r=append(r,u)
    }
    return r
}

func register_chain_ino(db_link *sql.DB,folder string,root_dir string,delim string){
    lst :=folder_split(folder,root_dir,delim)
    is_root :=false
    for _,item :=range(lst){
        if item==root_dir{
            is_root = true
        }
        refresh_folder(db_link,item,is_root)
        is_root =false
    }
}

func ensure_folder(folder *string,delim string){
    if len(*folder)==0{
        return
    }
    if !strings.HasSuffix(*folder,delim){
        *folder = *folder +delim
    }
    return
}

func mime_decode(rs_type int) string{
    coden_tab:=map[int]string{
        1:    "image/png",
        2: "image/jpeg",
        3: "image/gif",
        4: "image/tiff",
        5: "image/bmp",
        6: "image/svg+xml",
        7: "image/webp",       
        30: "text/plain",// # text
        31: "text/html",
        32: "text/xml", //#svg
        40: "application/json",
        41: "application/pdf",
        42: "application/msword",
        100:"application/octet-stream",
    }
    r,ok:=coden_tab[rs_type]
    if !ok{
        return "application/octet-stream"
    }
    return r
}

func mime_encode(input string)int{
    coden_tab:=map[string]int{
        "image/png":1,
        "image/jpeg":2,
        "image/gif":3,
        "image/tiff":4,
        "image/bmp":5,
        "image/svg+xml":6,
        "image/webp":7,       
        "text/plain":30,// # text
        "text/html":31,
        "text/xml":32, //#svg
        "application/json":40,
        "application/pdf":41,
        "application/msword":42,
        "application/octet-stream":100,
    }
    r,ok:=coden_tab[input]
    if !ok{
        return 100
    }
    return r
}

func ext_to_mime(str string)string{
    var r string
    switch str{
    case "jpeg","jpg","jpe":
        r= "image/jpeg"
    case "png":
        r= "image/png"
    case "gif":
        r = "image/gif"
    case "tif":
        r = "image/tiff"
    case "bmp":
        r ="application/x-bmp"
    case "webp":
        r ="image/webp"
    case "txt","text","fna","fasta","seq":
        r="text/plain"
    case "c","cpp","h","rb","sh","pl","php","js","py","cr","html","css","ini","R","md","xml":
        r="text/plain"
    case "svg":
        r="text/xml"
    case "mp3":
        r="audio/mp3"
    case "mp4":
        r="video/mpeg4"
    default:
        r="application/octet-stream"
    }
    return r
}

func relative_path_of(url string,root_dir string)string{
    if (strings.HasPrefix(url,root_dir)){
        return url[len(root_dir):len(url)]
    }
    return url
}
func path_file_name(url string,delim string)string{
    if(strings.Contains(url,delim)){
        if strings.LastIndex(url,delim) ==len(url)-1{
            return ""
        }else{
            return url[(strings.LastIndex(url,delim)+1):len(url)]
        }
    }    
    return url
}

func path_dir_name(url string, delim string)string{
    if(strings.Contains(url,delim)){
        return url[0:(strings.LastIndex(url,delim)+1)]
    }    
    return ""
}

func get_now_string()string{
    now:=time.Now()
    year := now.Year()     //年
    month := now.Month()   //月
    day := now.Day()       //日
    hour := now.Hour()     //小时
    minute := now.Minute() //分钟
    second := now.Second() //秒
    return fmt.Sprintf("%d-%02d-%02d %02d:%02d:%02d", year, month, day, hour, minute, second)
}

func str_shrink(input string,max_len int)string{
    var result string
    if len(input)<max_len{
        return input
    }
    if strings.Contains(input,"."){
        dot_rindex:= strings.LastIndex(input,".")
        post_fix_len :=len(input)-(dot_rindex+1)
        left_len :=max_len-(post_fix_len +1 + 3 + 4)
        result = input[0:(left_len)]+"..."+input[(dot_rindex-4):dot_rindex]
        if dot_rindex+1 == len(input){
            result=result+"." // ending with .
        }else{
            result =result+input[(dot_rindex):(len(input))]   
        }
    }else{
        left_len := max_len - (+1 +3 + 4)
        result = input[0:(left_len)]+"..."+input[len(input)-5:len(input)]
    }   
    return result
}


func set_intersect(a []string,b []string)[]string{
    var r []string
    temp_map :=make(map[string]bool)
    for _,v :=range(b){
        temp_map[v]=true
    }
    for _,v := range(a){
        _,ok := temp_map[v]
        if ok{
            r = append(r,v)
        }
    }
    return r
}

func set_substract(a []string,b []string)[]string{
    var r []string
    temp_map :=make(map[string]bool)
    for _,v :=range(b){
        temp_map[v]=true
    }
    for _,v := range(a){
        _,ok := temp_map[v]
        if !ok{
            r = append(r,v)
        }
    }
    return r
}

func file_suffix(file_name string)string{
    if strings.Contains(file_name,"."){
        idx := strings.LastIndex(file_name,".")
        return strings.ToLower(file_name[(idx+1):len(file_name)])
    }
    return ""
}

func dev_ino_uint64(dev_ino string)(uint64,uint64,error){
    reg:=regexp.MustCompile(`(\d+)_(\d+)`)
    mats :=reg.FindStringSubmatch(dev_ino)
    if len(mats)<3{
        return 0,0,errors.New("no matching format")
    }
    device_id, err :=strconv.ParseUint(mats[1],10,64)
    if err !=nil{
        return 0,0,errors.New("device_id converting error")
    }

    ino,err:= strconv.ParseUint(mats[2],10,64)
    if err !=nil{
        return 0,0,errors.New("ino converting error")
    }
    return device_id,ino,nil
}

func calc_pages(count int64,page_len int) int{
    var page_count int
    if int(count)%page_len==0{
        page_count = int(count)/page_len
    }else{
        page_count = int(count)/page_len+1
    }
    return page_count
}

func get_host_name()string{
    host_name,err:=os.Hostname()
    if err !=nil{
        host_name = ""
    }
    return host_name
}

func get_db(db_file string) (*sql.DB,error){
    // fmt.Println("Opening a database link")
    db, err := sql.Open("sqlite3",db_file)
    if err !=nil{
        return db,err
    }
    return db,nil
}

//====================================================================================================
// for blob
func random_str(n int) string{
    var buf bytes.Buffer
    str :="abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPSTUVWXYZ1234567890"
    max := len(str)-1
    for i:=0;i<n;i++{
        rand.Seed(time.Now().UnixNano())
        idx :=rand.Intn(max)
        buf.WriteByte(str[idx])
    }
    return buf.String()
}
func tag_gen(db_link *sql.DB)(string,error){
    tag :=random_str(10)
    table := new(Db_table)
    table.set_name("tags")
    table.add_column("tag_str",true)
    table.set("tag_str",tag)
    _,err := do_insert(db_link,table.pack_insert())
    if err !=nil{
        return tag,err
    }
    return tag,nil
}

func get_blob_file_page(path string,size_limit int64)(string,error){
    fileInfos,err := ioutil.ReadDir(path)
    if err !=nil{
        return "", err
    }
    reg := regexp.MustCompile(`blob(\d+)\.db`)
    max:=0
    var max_size int64 =0    
    for _,info:= range(fileInfos){
        m :=reg.FindStringSubmatch(info.Name())
        if len(m)>0{
            i,err:=strconv.Atoi(m[1])
            if err!=nil{
                return "",err
            }
            if  i>max{
                max = i
                max_size = info.Size()
            }
        }
    }
    if max == 0{
        return "1",nil
    }
    if max > 0 && max_size > size_limit{ // 1M
        return strconv.Itoa(max+1),nil
    }
    return  strconv.Itoa(max),nil
}

func blob_create_file(file_name string)(bool,error){
    db, err := sql.Open("sqlite3",file_name)
    sql_str:="CREATE TABLE blob_obj(id integer primary key autoincrement,type TINYINT UNSIGNED,tag CHAR(10),data blob);"
    sql_str +="CREATE INDEX blob_idx ON blob_obj(tag);"
    fmt.Println(sql_str)
    _, err = db.Exec(sql_str)
    if err != nil {
        return false,err
    }
    db.Close()
    return true,nil
}

func blob_save(db_file string,tag string,bin_data []byte,file_type int)(string,error){
    db, err := sql.Open("sqlite3",db_file)
    sql_str:="Insert into blob_obj(tag,type,data)values(?,?,?)"
    sql_run, err := db.Prepare(sql_str)
    if err !=nil{
        fmt.Println("?? error in db operation:")
        fmt.Println(sql_str)
        return tag,err
    }
    _, err =sql_run.Exec(tag,file_type,bin_data)
    if err !=nil{
        return tag,err
    }
    db.Close()
    return tag,nil
}

func blob_update(db_file string,tag string,bin_data []byte,file_type int)(bool,error){
    db, err := sql.Open("sqlite3",db_file)
    sql_str:="Update blob_obj set data=?,type=? where tag=?"
    sql_run, err := db.Prepare(sql_str)
    if err !=nil{
        fmt.Println("?? error in db operation:")
        fmt.Println(sql_str)
        return false,err
    }
    _, err =sql_run.Exec(bin_data,file_type,tag)
    if err !=nil{
        return false,err
    }
    db.Close()
    return true,nil
}


func blob_read(db_file string,tag string)(int,[]byte,error){
    db,err :=sql.Open("sqlite3",db_file)
    sql_str :="select type,data from blob_obj where tag=\""+tag+"\""
    rows,err :=db.Query(sql_str)
    if err !=nil{
        return 0,[]byte{},err
    }
    rows.Next()
    var rs_type int
    var data []byte
    err = rows.Scan(&rs_type,&data)
    defer rows.Close()
    db.Close()
    return rs_type,data,nil
}

func blob_save_file(db_file string,tag string,bin_file string,file_type int)(string,error){
    bin_handler, err := os.Open(bin_file)
    defer  bin_handler.Close()
    if err!=nil{
        return tag,err
    }
    bin_data,err:=ioutil.ReadAll(bin_handler)
    if err !=nil{
        return tag,err
    }
    _,err=blob_save(db_file,tag,bin_data,file_type)
    if err !=nil{
        return tag,err
    }
    return tag,nil
}

func blob_delete(db_file string,tag string)(bool,error){
    db,err :=sql.Open("sqlite3",db_file)
    sql_del := "delete from blob_obj where tag=\""+tag+"\""
    sql_run, err := db.Prepare(sql_del)
    if err !=nil{
        fmt.Println("?? error in db operation:")
        fmt.Println(sql_del)
        return false,err
    }
    _, err =sql_run.Exec()
    if err !=nil{
        return false,err
    }
    db.Close()
    return true,nil
}
// ====================================================================================================
// for resource

func resource_deposite(db_link *sql.DB,name string,rs_type int,data []byte,db_folder string)(string,error){
    tab_resource:=new(Db_table)
    tab_resource.set_name("resource")
    tab_resource.add_column("tag",true).add_column("page",false).add_column("name",true)
    tab_resource.add_column("type",false).add_column("rs_date",true).add_column("ref_count",false)
    page,err:=get_blob_file_page(db_folder,50000000)
    if err !=nil{
        return "",err
    }
    blob_file := db_folder + "blob"+page+".db"

    if ok,_:=file_exists(blob_file);!ok{
        _,err := blob_create_file(blob_file)
        if err !=nil{
            return "",err
        }
     }

    tag,err :=tag_gen(db_link)
    if err !=nil{
        // fmt.Println("tag generation failed")
        return "",err
    }


    _,err=blob_save(blob_file,tag,data,rs_type)
    if err !=nil{
        // delete the tag first [missing here]
        // then return
        return "",err
    }

    tab_resource.set("tag",tag).set("page",page).set("name",name).set("type",strconv.Itoa(rs_type))
    tab_resource.set("ref_count","0").set("rs_date",get_now_string())

    _,err=do_insert(db_link,tab_resource.pack_insert())
    if err !=nil{
        return "",err
    }
    return tag,nil
}
func resource_deposite_file(db_link *sql.DB,name string,rs_type int,file_name string,db_folder string)(string,error){
    handler, err := os.Open(file_name)
    defer  handler.Close()
    if err!=nil{
        return "",err
    }
    data,err:=ioutil.ReadAll(handler)
    if err !=nil{
        return "",err
    }
    tag,err :=resource_deposite(db_link,name,rs_type,data,db_folder)
    return tag,err
}

func resource_ref_count_inc(db_link *sql.DB,tag string)(bool,error){
    tab_resource:=new(Db_table)
    tab_resource.set_name("resource").add_column("tag",true).add_column("ref_count",false)
    tab_resource.set("ref_count","ref_count+1").set("tag",tag)
    check:=[]string{"tag"}
    _,err := do_update(db_link,tab_resource.pack_update(check))
    if err !=nil{
        return false,err
    }
    return true,nil
}

func resource_ref_count_dec(db_link *sql.DB,tag string)(bool,error){
    tab_resource:=new(Db_table)
    tab_resource.set_name("resource").add_column("tag",true).add_column("ref_count",false)
    tab_resource.set("ref_count","ref_count-1").set("tag",tag)
    check:=[]string{"tag"}
    _,err := do_update(db_link,tab_resource.pack_update(check))
    if err !=nil{
        return false,err
    }
    return true,nil
}
func resource_update_name(db_link *sql.DB,tag string,name string)(bool,error){
    tab_resource:=new(Db_table)
    tab_resource.set_name("resource").add_column("tag",true).add_column("name",true)
    tab_resource.set("name",name).set("tag",tag)
    check:=[]string{"tag"}
    _,err := do_update(db_link,tab_resource.pack_update(check))
    if err !=nil{
        return false,err
    }
    return true,nil
}

func resource_update_type(db_link *sql.DB,tag string,rs_type int)(bool,error){
    tab_resource:=new(Db_table)
    tab_resource.set_name("resource").add_column("tag",true).add_column("type",false)
    tab_resource.set("type",strconv.Itoa(rs_type)).set("tag",tag)
    check:=[]string{"tag"}
    _,err := do_update(db_link,tab_resource.pack_update(check))
    if err !=nil{
        return false,err
    }
    return true,nil
}

func get_resource_record(db_link *sql.DB,tag string)(Resource_record,error) {
    tab_resource:=new(Db_table)
    tab_resource.set_name("resource").add_column("page",false).add_column("tag",true)
    tab_resource.add_column("type",false).add_column("rs_date",true).add_column("ref_count",false)
    tab_resource.set("tag",tag)
    var record Resource_record
    cnt,err :=do_count(db_link,tab_resource.pack_count("cnt"))
    if err !=nil{
        return record,err
    }
    if cnt <1{
        return record,errors.New("no record")
    }
    rows,err:=db_link.Query(tab_resource.pack_select("name,page,type,rs_date,ref_count","",""))
    if err !=nil{
        return record,err
    }
    rows.Next()
    err=rows.Scan(&record.Name, &record.Page, &record.Rs_type, &record.Rs_date,&record.Ref_count)
    record.Tag = tag
    defer    rows.Close()
    if err !=nil{
        return record,err
    }
    return record,nil
}


func get_image(db_link *sql.DB, db_folder string,tag string)(int,[]byte,error){
    tab_resource:=new(Db_table)
    tab_resource.set_name("resource")
    tab_resource.add_column("tag",true).add_column("page",false).add_column("name",true)
    tab_resource.add_column("type",false).add_column("rs_date",true).add_column("ref_count",false)

    tab_resource.set("tag",tag)
    page,err:=do_select_id(db_link,tab_resource.pack_select("page","",""))
    if err !=nil{
        fmt.Printf("sql:%s,error:%#v",tab_resource.pack_select("page","",""),err)
        return 0,[]byte{},err
    }
    blob_file := db_folder +"blob"+strconv.FormatInt(page[0],10)+".db"
    rs_type,data,err :=blob_read(blob_file,tag)
    if err !=nil{
        return 0,[]byte{},err
    }
    return rs_type,data,nil
}

func get_image_mime(db_link *sql.DB, tag string)(string,error){
    tab_resource:=new(Db_table)
    tab_resource.set_name("resource").add_column("tag",true)
    tab_resource.set("tag",tag)
    rs_type,err:=do_select_id(db_link,tab_resource.pack_select("rs_type","",""))
    if err!=nil || len(rs_type)==0{
        return mime_decode(0),err
    }
    return mime_decode(int(rs_type[0])),nil
}

func get_text(db_link *sql.DB, db_folder string,tag string)(int,string,error){
    rs_type,data,err:=get_image(db_link,db_folder,tag)
    var result string
    if len(data) >0{
        result = string(data)
    }
    return rs_type,result,err
}

func image_count(db_link *sql.DB)(int64,error){
    tab_resource :=new(Db_table)
    tab_resource.set_name("resource")
    tab_resource.add_column("type<",false)
    tab_resource.set("type<","10")
    cnt,err:=do_count(db_link,tab_resource.pack_count("cnt"))
    return cnt,err 
}
func list_images(db_link *sql.DB, page_len int,page int) []Resource_record{
    var result =  []Resource_record{}
    if page_len <1{
        return result
    }
    cnt,err := image_count(db_link)
    if err!=nil || cnt==0{
        return result
    }
    var page_count int64
    if cnt%int64(page_len)==0{
        page_count = cnt/int64(page_len)
    }else{
        page_count = cnt/int64(page_len)+1
    }
    if page>int(page_count){
        page = int(page_count)
    }
    if page <1{
        page =1
    }
    start :=(page-1)*page_len

    tab_resource :=new(Db_table)
    tab_resource.set_name("resource")
    tab_resource.add_column("tag",true).add_column("type<",false).add_column("name",true)
    tab_resource.add_column("rs_date",true).add_column("ref_count",false)

    tab_resource.set("type<","10")
    rows,err :=db_link.Query(tab_resource.pack_select("tag,name,type,rs_date,ref_count","rsid desc",strconv.Itoa(start)+","+strconv.Itoa(page_len)))
    if err !=nil{
        return result
    }
    for rows.Next(){
        var item Resource_record
        rows.Scan(&item.Tag,&item.Name,&item.Rs_type,&item.Rs_date,&item.Ref_count)
        result = append(result,item)
    }
    return result
}

func extract_tags(text string)[]string{
    if !strings.Contains(text,"get_image/"){
        return []string{}
    }
    var result []string
    fmt.Println(text)
    reg := regexp.MustCompile(`<img src="\.?\.?/?get_image/([\w\d]+)"`)
    found :=reg.FindAllStringSubmatch(text,-1)
    for _,r := range found{
        result = append(result,r[1])
    }
    return result
}

func extract_img_names(text string)map[string]string{
    var result =make(map[string]string)
    if !strings.Contains(text,"get_image/"){
        return result
    }
    
    reg := regexp.MustCompile(`<img src="\.?\.?/?get_image/([\w\d]+)" alt="([\w\d]+)" `)
    found :=reg.FindAllStringSubmatch(text,-1)
    for _,r := range found{
        result[r[1]] = r[2]
    }
    return result
}

func resource_delete(db_link *sql.DB,tag string,db_folder string)(bool,error){
    tab_resource:=new(Db_table)
    tab_resource.set_name("resource").add_column("tag",true)
    tab_resource.set("tag",tag)
    cnt,err := do_count(db_link,tab_resource.pack_count("cnt"))
    if err !=nil{
        return false,errors.New("record count error")
    }
    if cnt ==0{
        return false,errors.New("no record error")
    }
    resource_record,err :=get_resource_record(db_link,tag )
    if err !=nil{
        return false,err
        // return false,errors.New("record fetch error")
    }
    page :=resource_record.Page
    blob_file := db_folder+"blob"+strconv.Itoa(page)+".db"
    ok,err:=blob_delete(blob_file,tag)
    if err !=nil{
        return false,err
    }
    if !ok{
        return false,err
    }
    _,err =do_delete(db_link,tab_resource.pack_delete())
    if err !=nil{
        return false,err
    }
    return true,nil
}

func resource_update(db_link *sql.DB,tag string,rs_type int,data []byte,db_folder string)(bool,error){
    tab_resource:=new(Db_table)
    tab_resource.set_name("resource").add_column("tag",true)
    tab_resource.set("tag",tag)
    cnt,err := do_count(db_link,tab_resource.pack_count("cnt"))
    if err !=nil{
        return false,errors.New("record count error")
    }
    if cnt ==0{
        return false,errors.New("no record error")
    }
    resource_record,err :=get_resource_record(db_link,tag )
    if err !=nil{
        return false,err
    }
    page :=resource_record.Page
    // rs_type :=resource_record.Rs_type
    blob_file := db_folder+"blob"+strconv.Itoa(page)+".db"
    ok,err:=blob_update(blob_file,tag,data,rs_type)
    if err !=nil{
        return false,err
    }
    if !ok{
        return false,err
    }
    resource_update_type(db_link,tag,rs_type)
    return true,nil
}


// resource link======================================================================================
func resource_link_add(db_link *sql.DB,tag string, app int, app_tag string)(bool,error){
    tab := new(Db_table)
    tab.set_name("resource_link").add_column("tag",true).add_column("app",false).add_column("app_tag",true)
    tab.set("tag",tag).set("app",strconv.Itoa(app)).set("app_tag",app_tag)
    _,err :=do_insert(db_link,tab.pack_insert())
    if err !=nil{
        return false,err
    }
    return true,nil
}

func resource_link_del(db_link *sql.DB,tag string, app int, app_tag string)(bool,error){
    tab := new(Db_table)
    tab.set_name("resource_link").add_column("tag",true).add_column("app",false).add_column("app_tag",true)
    tab.set("tag",tag).set("app",strconv.Itoa(app)).set("app_tag",app_tag)
    _,err :=do_insert(db_link,tab.pack_delete())
    if err !=nil{
        return false,err
    }
    return true,nil
}

func resource_link_read(db_link *sql.DB,tag string)(int, string, error){
    tab := new(Db_table)
    tab.set_name("resource_link").add_column("tag",true)
    tab.set("tag",tag)
    var app int
    var app_tag string
    rows,err:=db_link.Query(tab.pack_select("app,app_tag","",""))
    if err !=nil{
        return 0,"",err
    }
    for rows.Next(){
        rows.Scan(&app,&app_tag)
    }
    if app !=0{ // let's assume there is only one reference
        return app,app_tag,nil
    }
    return 0,"",errors.New("no record")
}

//====================================================================================================
// for file_note
//====================================================================================================
func add_note(db_link *sql.DB,host_name string,device_id string,ino string,note string,color string,root_dir string,db_folder string)(string,error){
    tab_note:=new(Db_table)
    tab_note.set_name("file_note")
    tab_note.add_column("tag",true).add_column("file_dir",true).add_column("file_name",true).add_column("tag",true)
    tab_note.add_column("note",true).add_column("color",false).add_column("ndate",true)
    device_id_uint64,err :=strconv.ParseUint(device_id,10,64)
    delim:=sys_delim()
    if err !=nil{
        return "",err
    }
    ino_uint64,_:=strconv.ParseUint(ino,10,64)
    if err !=nil{
        return "",err
    }
    url,err := file_url(db_link,device_id_uint64,ino_uint64,100,delim)
    if err !=nil{
        return "",err
    }
    relative_url:=relative_path_of(url,root_dir)
    if delim == "\\"{
        // on windows
        // in the file_note table, the standard deliminator is "/" 
        relative_url = strings.ReplaceAll(relative_url, "\\","/")
    }
    file_name := path_file_name(relative_url,delim)
    file_dir := path_dir_name(relative_url,delim)
    tag,err := tag_gen(db_link)
    if err !=nil{
        // delete the tag
        return tag,err
    }
    blob_tag,err:=resource_deposite(db_link,"0x_text_"+get_now_string(),33,[]byte(note),db_folder)
    if err !=nil{
        // delete the tag
        return tag,err
    }
    tab_note.set("note","#<0x_"+blob_tag+"_>").set("color",color)    
    tab_note.set("file_name",file_name).set("file_dir",file_dir).set("tag",tag).set("ndate",get_now_string())
    
    _,err=do_insert(db_link,tab_note.pack_insert())
    if err !=nil{
        return tag,err
    }
    resource_ref_count_inc(db_link,blob_tag)
    return tag,nil    
}



func get_note_record(db_link *sql.DB,file_dir string, file_name string) (Note_record,error){
    var result Note_record

    tab_note:=new(Db_table)
    tab_note.set_name("file_note")
    tab_note.add_column("tag",true).add_column("file_dir",true).add_column("file_name",true).add_column("tag",true)
    tab_note.add_column("note",true).add_column("color",false).add_column("ndate",true)

    tab_note.set("file_dir",file_dir).set("file_name",file_name)
    rows,err :=db_link.Query(tab_note.pack_select("tag,file_dir,file_name,note,ndate,color","",""))
    defer rows.Close()
    if err !=nil{
        return result,err
    }
    if rows.Next(){
        rows.Scan(&result.Tag,&result.File_dir, &result.File_name, &result.Note,&result.Ndate,&result.Color)
        return result,nil
    }
    return result,errors.New("no record")
}

func get_note_by_tag(db_link *sql.DB,tag string)(Note_record,error){
    var result Note_record
    tab_note:=new(Db_table)
    tab_note.set_name("file_note").add_column("tag",true)

    tab_note.set("tag",tag)
    rows,err :=db_link.Query(tab_note.pack_select("tag,file_dir,file_name,note,ndate,color","",""))
    defer rows.Close()
    if err !=nil{
        return result,err
    }
    if rows.Next(){
        rows.Scan(&result.Tag,&result.File_dir, &result.File_name, &result.Note,&result.Ndate,&result.Color)
        return result,nil
    }
    return result,errors.New("no record")

}


func notes_count(db_link *sql.DB)(int64,error){
    tab_note:=new(Db_table)
    tab_note.set_name("file_note")
    cnt,err :=do_count(db_link,tab_note.pack_count("cnt"))
    return cnt,err
}

func list_notes_record(db_link *sql.DB,page_len int,page int)([]Note_record,error){
    var result []Note_record
    if page_len <1{
        return result,errors.New("page len error")
    }
    cnt,err := notes_count(db_link)
    if err!=nil || cnt==0{
        return result,errors.New("count error")
    }
    page_count := calc_pages(cnt, page_len)
 
    if page>int(page_count){
        page = int(page_count)
    }
    if page <1{
        page =1
    }
    start :=(page-1)*page_len

    tab_note := new(Db_table)
    tab_note.set_name("file_note")
    rows,err :=db_link.Query(tab_note.pack_select("tag,file_dir,file_name,note,ndate,color","nid desc",strconv.Itoa(start)+","+strconv.Itoa(page_len)))
    if err !=nil{
        return result,err
    }    
    var row Note_record
    for rows.Next(){
        rows.Scan(&row.Tag,&row.File_dir,&row.File_name,&row.Note,&row.Ndate,&row.Color)
        result =append(result,row)
    }
    return result,nil
}
func list_notes(db_link *sql.DB,page_len int,page int,db_folder string)([]Note_record,error){
    var result =  []Note_record{}
    result,err := list_notes_record(db_link,page_len,page)
    if err!=nil{
        return result, err
    }
    reg :=regexp.MustCompile(`#<0x_([\d\w]+)_>`)
    for i:=0;i<len(result);i++{
        row :=&result[i]
        row.Color_str=color_decode(row.Color)
        mats :=reg.FindStringSubmatch(row.Note)
        if len(mats)>1{
            text_tag := mats[1]
            _,real_note,err :=get_text(db_link,db_folder,text_tag)
            if err ==nil{
                row.Note = real_note
            }
        }
    }
    return result,nil
}



func del_note(db_link *sql.DB,file_dir string, file_name string,db_folder string)(bool,error){
    if sys_delim()=="\\"{
        file_dir =strings.ReplaceAll(file_dir,"\\","/")
        file_name=strings.ReplaceAll(file_name,"\\","/")
    }
    record,err := get_note_record(db_link,file_dir,file_name)
    if err !=nil{
        return false,err
    }
    reg:=regexp.MustCompile(`#<0x_([\d\w]+)_>`)
    mats:= reg.FindStringSubmatch(record.Note)
    // there are others to delete:
    // 1. the resource related to this note
    // 2. the blob
    if len(mats)>1{
        text_tag := mats[1]
        _,note_text,err :=get_text(db_link, db_folder ,text_tag )
        if err ==nil{
            res_tags := extract_tags(note_text)
            for _,tag := range(res_tags){
                resource_ref_count_dec(db_link,tag)
                resource_link_del(db_link,tag,1,record.Tag)
            }
            // delete the text_resouce
            _,err:=resource_delete(db_link,text_tag,db_folder)
            if err !=nil{
                fmt.Printf("error:%#v",err)
                return false,err
            }
            
        }
    }
    tab_note:=new(Db_table)
    tab_note.set_name("file_note").add_column("file_dir",true).add_column("file_name",true)
    tab_note.set("file_dir",file_dir).set("file_name",file_name)
    _,err=do_delete(db_link,tab_note.pack_delete())
    if err !=nil{
        fmt.Printf("error:%#v",err)
        return false,err
    }
    return true,nil
}

func edit_note(db_link *sql.DB,tag string,note string,color string,db_folder string)(bool,error){
    record,err := get_note_by_tag(db_link,tag)
    fmt.Printf("%#v",tag)
    if err !=nil{
        fmt.Printf("%#v",err)
        return false,err
    }
    reg:=regexp.MustCompile(`#<0x_([\d\w]+)_>`)
    mats:= reg.FindStringSubmatch(record.Note)
    if len(mats)>1{
        text_tag := mats[1]
        _,note_text,err :=get_text(db_link, db_folder ,text_tag )
        if err ==nil{
            old_tags := extract_tags(note_text)
            new_tags := extract_tags(note)
            new_tags_map :=extract_img_names(note)
            tags_to_update:=set_intersect(old_tags,new_tags)
            tags_to_delete:=set_substract(old_tags,tags_to_update)
            tags_to_insert:=set_substract(new_tags,tags_to_update)
            
            // fmt.Printf("old_tags:%#v\n",old_tags)
            // fmt.Printf("old_tags:%#v\n",new_tags)

            // fmt.Printf("to_update:%#v\n",tags_to_update)
            // fmt.Printf("to_del:%#v\n",tags_to_delete)
            // fmt.Printf("to_insert:%#v\n",tags_to_insert)

            //update the names of the images in the new note
            for _,item :=range(new_tags){
                item_name,ok:=new_tags_map[item]
                if ok{
                    if item_name == ""{
                        continue
                    }
                    resource_update_name(db_link,item,item_name)
                }
            }

            for _,item :=range(tags_to_insert){
                resource_ref_count_inc(db_link,item)
                resource_link_add(db_link,item,1,tag)
            }

            //only decrease the ref_count, break the resource_link
            for _,item :=range(tags_to_delete){
                resource_ref_count_dec(db_link,item)
                resource_link_del(db_link,item,1,tag)
            }
        }
        // update the text resource
        _,err=resource_update(db_link,text_tag,33,[]byte(note),db_folder)
        if err !=nil{
            return false,err
        }
    }

    tab_note:=new(Db_table)
    tab_note.set_name("file_note")
    tab_note.add_column("tag",true).add_column("color",false)
    
    new_color,_:=strconv.Atoi(color)
    if record.Color != new_color{
        tab_note.set("tag",tag).set("color",color)
        check:=[]string{"tag"}
        _,err = do_update(db_link,tab_note.pack_update(check))
    }   
    if err!=nil{
        return false,err
    }
    return true,nil
}

func note_update_name(db_link *sql.DB,file_dir string, file_name string,new_name string)(bool,error){
    if sys_delim() =="\\"{
        file_dir =strings.ReplaceAll(file_dir,"\\","/")
    }
    
    note,err:=get_note_record(db_link,file_dir, file_name)
    if err!=nil {
        return false, err
    }
    tab_note:=new(Db_table)
    tab_note.set_name("file_note")
    tab_note.add_column("tag",true).add_column("file_name",true).add_column("tag",true)
    tab_note.set("file_name",new_name).set("tag",note.Tag)
    check:=[]string{"tag"}
    _,err= do_update(db_link,tab_note.pack_update(check))
    if err !=nil{
        return false, err
    }
    return true,nil
}

func get_note_map(db_link *sql.DB,device_id uint64,ino uint64,root_dir string,db_folder string) (map[string]Note_record, error){
    tab_note:=new(Db_table)
    tab_note.set_name("file_note")
    tab_note.add_column("tag",true).add_column("file_dir",true).add_column("file_name",true).add_column("tag",true)
    tab_note.add_column("note",true).add_column("color",false).add_column("ndate",true)
    delim :=sys_delim()
    result := make(map[string]Note_record)
    this_url,err:=file_url(db_link,device_id,ino,100,delim)
    if err !=nil{
        return result,err
    }

    rel_file_url:=relative_path_of(this_url,root_dir)
    rel_file_dir := path_dir_name (rel_file_url,delim)
    if delim=="\\"{
        rel_file_dir=strings.ReplaceAll(rel_file_dir,"\\","/")
    }

    tab_note.set("file_dir",rel_file_dir)
    rows, err := db_link.Query(tab_note.pack_select("tag,file_name,note,color","",""))
    defer rows.Close()
    if err !=nil{
        return result,err
    }
    reg :=regexp.MustCompile(`#<0x_([\d\w]+)_>`)
    for rows.Next(){
        var fnv Note_record
        rows.Scan(&fnv.Tag,&fnv.Name,&fnv.Note,&fnv.Color)        
        mats := reg.FindStringSubmatch(fnv.Note)
        if len(mats)>1{
            _,real_note,err:=get_text(db_link, db_folder,mats[1])
            if err==nil{
                fnv.Note=real_note
            }
        }
        result[fnv.Name]=fnv
    }
    return result,nil
}


func color_decode(id int) string{
    coden_tab :=map[int]string{1:"green",2:"red",3:"blue",4:"purple",5:"orange",6:"yellow",7:"grey"}
    color,ok :=coden_tab[id]
    if !ok{
        return "default"
    }
    return color
}


func rebuild(db_link *sql.DB,root_dir string)(bool, error){
    page_len := 50
    count,err := notes_count(db_link)
    if err !=nil{
        return false, err
    }
    all_paths :=make(map[string]bool)
    pages := calc_pages(count, page_len)
    for i:=0;i<pages;i++{
        notes,err :=list_notes_record(db_link,page_len,(i+1))
        if err !=nil{
            continue
        }
        for _,nt:=range(notes){
            // register_ino(db,)
            paths :=folder_split(root_dir+nt.File_dir+nt.File_name,root_dir,sys_delim())
            for _,path :=range(paths){
                all_paths[path]=true
            }
        }
    }

    for path,_ :=range(all_paths){
        is_root:=false
        if path == root_dir{
            is_root= true
        }

        fnode,err := get_Fnode(path,is_root)
        if err !=nil{
            continue
        }
        _,err=register_ino(db_link,fnode)
        if err !=nil{
            break
        }
    }
    if err !=nil{
        return false, err
    }
    return true,err

}


func file_rename(db_link *sql.DB,device_id uint64,ino uint64,new_name string,root_dir string)(bool,error){
    delim :=sys_delim()
    old_url,err :=file_url(db_link,device_id,ino, 100,delim )
    if err !=nil{
        return false,err
    }
    old_name := path_file_name(old_url,delim)
    dir := path_dir_name(old_url,delim)
    new_path := dir + new_name
    err = os.Rename(old_url,new_path)
    if err !=nil{
        return false,err
    }
    // handle ino tree

    new_fnode,err :=get_Fnode(new_path,false)
    if err !=nil{
        return false,err
    }
    _,err=update_ino(db_link,new_fnode)
    if err !=nil{
        return false,err
    }

    // handle file_note
    file_dir :=relative_path_of(dir,root_dir)

    _,err=note_update_name(db_link,file_dir,old_name,new_name)
    fmt.Println("url:",old_url)
    fmt.Println("old_name:",old_name)
    fmt.Println("file_dir:",file_dir)
    fmt.Println("new_name",new_name)    
    

    if err !=nil{
        if err.Error()=="no record"{
            return true,nil
        }else{
            return false, err
        }       
    }
    return true,nil  
}

// for gin view--------------------------------------------------------
func Fnode_to_view(node *Fnode) Fnode_view{
    var result Fnode_view
    result.Name,result.IsDir, result.Dev, result.Ino=node.Name,node.IsDir, node.Dev, node.Ino
    result.Parent_dev, result.Parent_ino =node.Parent_dev,node.Parent_ino
    return result
}

func unescapeHtmlTag(input string)template.HTML{
    return template.HTML(input)
}

func draw_page_bar(page_count int, curr_page int,curr_style string,jump_url string)string{
    r :=""
    for i:=1;i<page_count+1;i++{
        if i==curr_page{
            r +="              <span class='layui-laypage-curr'><em class='layui-laypage-em' style='"+curr_style+"'></em><em>"+strconv.Itoa(curr_page)+"</em></span>\n"
        }else{
            r +="              <a href='"+jump_url+strconv.Itoa(i)+"'>"+strconv.Itoa(i)+"</a>\n"
        }
    }
    return r
}

//=====================================================================
// for shortcut
func add_shortcut(db_link *sql.DB, file_dir string, file_name string, sc_type string)(bool,error){
	if sys_delim()=="\\"{
        file_dir=strings.ReplaceAll(file_dir,"\\","/")
    }    
    
    tab := new(Db_table)
	tab.set_name("shortcut").add_column("file_dir",true).add_column("file_name",true).add_column("file_dir",true).add_column("type",true)
	tab.set("file_dir",file_dir).set("file_name",file_name).set("type",sc_type)

	cnt,err := do_count(db_link,tab.pack_count("cnt"))
	if err!=nil{
		return false,err
	}
	if cnt>0{
		return false,nil
	}
	_,err =do_insert(db_link,tab.pack_insert())
	if err !=nil{
		return false, err
	}
	return true,nil
}

func del_shortcut(db_link *sql.DB, file_dir string, file_name string)(bool,error){
    if sys_delim()=="\\"{
        file_dir=strings.ReplaceAll(file_dir,"\\","/")
    }   

    tab := new(Db_table)
	tab.set_name("shortcut").add_column("file_dir",true).add_column("file_name",true).add_column("file_dir",true).add_column("type",true)
	tab.set("file_dir",file_dir).set("file_name",file_name)

	_,err :=do_delete(db_link,tab.pack_delete())
	if err !=nil{
		return false, err
	}
	return true,nil
}

func get_shortcut_map(db_link *sql.DB, file_url string, root_dir string)(map[string]string,error){
    tab := new(Db_table)
	tab.set_name("shortcut").add_column("file_dir",true).add_column("file_name",true).add_column("file_dir",true).add_column("type",true)
    rel_url:=relative_path_of(file_url,root_dir)
    file_dir := path_dir_name(rel_url,"/")
    if sys_delim()=="\\"{
        file_dir=strings.ReplaceAll(file_dir,"\\","/")
    }  
    tab.set("file_dir",file_dir)
    rows,err:=db_link.Query(tab.pack_select("file_name,type","",""))
    result:=make(map[string]string)
    if err!=nil{
        return result,err
    }
    for rows.Next(){
        var name,sc_type string
        rows.Scan(&name,&sc_type)
        if name==""{
            result["000root000"]=sc_type 
        }else{
            result[name]=sc_type
        }
    }
    return result,nil
}

 
func shortcut_entry(db_link *sql.DB,sc_type string,root_dir string)(string,error){
    tab := new(Db_table)
	tab.set_name("shortcut").add_column("file_dir",true).add_column("file_name",true).add_column("file_dir",true).add_column("type",true)
	tab.set("type",sc_type)
    var temp_list []string
    var file_dir string
    var file_name string
    var rst string 
    rows,err := db_link.Query(tab.pack_select("file_dir,file_name","",""))
    if err !=nil{
        return "",err
    }
    for rows.Next(){
        err:=rows.Scan(&file_dir,&file_name)
        if err !=nil{
            continue
        }
        is_root :=false
        if file_dir =="" && file_name==""{
            is_root=true
        }        
        fnode,err := get_Fnode(root_dir+file_dir+file_name,is_root)
        if err !=nil{
            continue
        }
        title:=""
       
        if sc_type=="f"{
            title=file_name
            rst="{'title':'"+title+"',"+"'id':"+strconv.FormatUint(fnode.Ino,10)
            rst = rst +",'href':'/list/"+strconv.FormatUint(uint64(fnode.Dev),10)+"_"+strconv.FormatUint(fnode.Parent_ino,10)
            rst = rst +"&"+strconv.FormatUint(uint64(fnode.Dev),10)+"_"+strconv.FormatUint(fnode.Ino,10)+"'}"
        }
        if sc_type =="d"{
            if is_root{
                title=root_dir
            }else{
                tmp_folder_name :=file_dir[0:(len(file_dir)-1)]
                title=path_file_name(tmp_folder_name,"/")
            }
            rst="{'title':'"+title+"',"+"'id':"+strconv.FormatUint(fnode.Ino,10)
            rst = rst +",'href':'/list/"+strconv.FormatUint(uint64(fnode.Dev),10)+"_"+strconv.FormatUint(fnode.Ino,10)+"'}\n"
        }
        temp_list=append(temp_list,rst)  
    }
    return "["+strings.Join(temp_list,",") +"]",nil  

//  child:[{  title:'Cleaving-ribozyme',id:1380,href:'/list/16777218_1380'  },{  title:'CRISPR',id:1131,href:'/list/16777218_1131'  }]

}

func shortcut_map_to_str(sc_map map[string]string,sc_type string) string{
    var s []string
    for k,v:=range(sc_map){
        if v==sc_type{
            s=append(s,k)   
        }
    }
    if len(s)>0{
        return strings.Join(s,",")
    }
    return ""
}

func sys_delim()string{
    return string(os.PathSeparator)
}
// ================ for database initialize ========================
func install_db(db_file string) (bool,error){
    db, err := sql.Open("sqlite3",db_file)
    if err !=nil{
        fmt.Println("error linking database driver")
        return false,err
    }
    sql_tables :=`
CREATE TABLE IF NOT EXISTS "ino_tree"( "id" INTEGER PRIMARY KEY AUTOINCREMENT, "host_name" VARCHAR(100),"device_id" BIGINT UNSIGNED,
    "ino" BIGINT UNSIGNED,"parent_ino" BIGINT UNSIGNED,"name" VARCHAR(250),"type" CHAR(1),"state" CHAR(1) );
CREATE TABLE IF NOT EXISTS "file_note"( "nid" INTEGER PRIMARY KEY AUTOINCREMENT, "tag" CHAR(10),"file_dir" VARCHAR(250),
    "file_name" VARCHAR(250),"note" TEXT,ndate DATETIME, color CHAR(1));
create table IF NOT EXISTS tags(tag_id INTEGER PRIMARY KEY AUTOINCREMENT,tag_str CHAR(10));
create table IF NOT EXISTS note_ino(tid INTEGER  PRIMARY KEY AUTOINCREMENT,tag CHAR(10),host_name VARCHAR(40),device_id VARCHAR(10), ino BIGINT UNSIGNED);
create table IF NOT EXISTS resource(rsid INTEGER PRIMARY KEY AUTOINCREMENT, page SMALLINT UNSIGNED, tag CHAR(10), name VARCHAR(250),type TINYINT UNSIGNED,rs_date DATETIME,ref_count TINYINT);
create table IF NOT EXISTS resource_link(rsl_id INTEGER PRIMARY KEY AUTOINCREMENT, tag CHAR(10), app TINYINT, app_tag CHAR(10));
create table IF NOT EXISTS shortcut(scid INTEGER PRIMARY KEY AUTOINCREMENT,tack_id INT,file_dir VARCHAR(250), file_name VARCHAR(250),type CHAR(1),order_id INTERGER);
create index IF NOT EXISTS idx_dev_ino on ino_tree(host_name,device_id, ino);
create index IF NOT EXISTS idx_dev_parent on ino_tree(host_name,device_id,parent_ino);
create index IF NOT EXISTS idx_dev_ino_note on file_note(tag);
create index IF NOT EXISTS idx_file_note_path on file_note(file_dir,file_name);
create index IF NOT EXISTS idx_resource_tag on resource(tag);
create index IF NOT EXISTS idx_resource_link_tag on resource_link(tag);
create index IF NOT EXISTS idx_tags on tags(tag_str);
`
    _, err = db.Exec(sql_tables)
    db.Close()
    if err != nil {
        fmt.Printf("error:%q\n", err)
        return false,err
    }
    return true,nil
}

var db_path = flag.String("d", "./Filegai", "database folder path")
var to_create_db = flag.Bool("n", false, "create the new database ")
var app_port =flag.Int("p",8080,"serving port,default 8080")
var expose_server =flag.Bool("e",false,"to expose the server to internet")
var app_usage =`usage: Filegai [options] Folder
-n: to create a new database
-d db_folder : the database folder, default ./Filegai
-e: to expose the server to internet. Dangerous!!, don't use, default No. 
-p number:the communication port
`

//-----------------------the MAIN FUNCTION---------------------------

func main(){
    if len(os.Args)<2{
        fmt.Println(app_usage)
        os.Exit(1) 
    }
    flag.Parse()
    if flag.NArg() ==0{
        fmt.Println("please provide the folder to serve")
        fmt.Println(app_usage)
        os.Exit(1)
    }    
    db_folder := *db_path
    system_delim := sys_delim()
    ensure_folder(&db_folder,system_delim)
    db_file :=db_folder+"Filegai.db"

    root_dir := get_abs_path(flag.Arg(0))
    // root_dir := flag.Arg(0)
    ensure_folder(&root_dir,system_delim)

    if ok,_ :=file_exists(root_dir);!ok{
        fmt.Printf("Serving folder [%s] does not exists,error %q\n",root_dir)
        fmt.Println(app_usage)
        os.Exit(1)
    }

    if *to_create_db{
        if ok,_:=file_exists(db_folder);ok{
            fmt.Println("folder already exists,please don't use -n option for existing database")
            fmt.Println(app_usage)
            os.Exit(1)
        }
        err := os.Mkdir(db_folder,0755)
        if err !=nil{
            fmt.Printf("error:make directory [%s] failed\n",db_folder)
            os.Exit(1)
        }
        _,err = install_db(db_file)
        if err !=nil{
            fmt.Printf("error:install database[%s] failed\n",db_file)
            os.Exit(1)
        }
    }else{
        if ok,_:=file_exists(db_folder);!ok{
            fmt.Printf("folder [%s] does not exists\n",db_folder)
            fmt.Println(app_usage)
            os.Exit(1)
        }
    }

    // fmt.Println("Opening a database link")
    db, err := get_db(db_file)
    if err !=nil{
        fmt.Println("?? error opening database file:",db_file)
        return
    }
    db.Close()
    fmt.Println("*********************************************************")
    fmt.Println("Serving:",root_dir)
    fmt.Println("Database folder:",db_folder)
    fmt.Println("Main Database file:",db_file)
    fmt.Println("*********************************************************")


    r := gin.Default()
    //定义路由的GET方法及响应处理函数
    
    r.Static("/public", "./public")
    r.SetFuncMap(template.FuncMap{
        "unescapeHtmlTag":unescapeHtmlTag,
    })
    // r.LoadHTMLFiles("templates/index.html", "templates/notes.html","templates/images.html",
    //                 "templates/show_code.html","templates/status.html")
    r.LoadHTMLGlob("templates/*")
    r.GET("/",func(c *gin.Context){  
        db, err = get_db(db_file)
        if err !=nil{
            fmt.Println("?? error opening database file:",db_file)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        defer db.Close()
       
        refresh_folder(db,root_dir,true)
        this_fnode,_ := get_Fnode(root_dir,true)
        c.HTML(http.StatusOK,"status.html",gin.H{
            "root_dir":root_dir,
            "this_fnode":this_fnode,
            "db_folder":db_folder,
            "db_file":db_file,
            "dev_ino":strconv.FormatUint(uint64(this_fnode.Dev),10)+"_"+strconv.FormatUint(this_fnode.Ino,10),
        })
        // c.Redirect(http.StatusTemporaryRedirect,"/list/"+strconv.FormatUint(uint64(this_fnode.Dev),10)+"_"+strconv.FormatUint(this_fnode.Ino,10))
    })
    r.GET("/list",func(c *gin.Context){ 
        db, err = get_db(db_file)
        if err !=nil{
            fmt.Println("?? error opening database file:",db_file)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        defer db.Close()

        refresh_folder(db,root_dir,true)
        this_fnode,_ := get_Fnode(root_dir,true)
        c.Redirect(http.StatusTemporaryRedirect,"/list/"+strconv.FormatUint(uint64(this_fnode.Dev),10)+"_"+strconv.FormatUint(this_fnode.Ino,10))
    })
    r.GET("/nav/:dev_ino",func(c *gin.Context){ 
        db, err = get_db(db_file)
        if err !=nil{
            fmt.Println("?? error opening database file:",db_file)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        defer db.Close()

        device_id, ino,err:=dev_ino_uint64(c.Param("dev_ino"))
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        url,err := file_url(db, device_id,ino,100,"/")
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        cmd := exec.Command("open",url)
        err = cmd.Start()
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        c.Redirect(http.StatusTemporaryRedirect,"/list/"+c.Param("dev_ino"))
      
    })
    r.GET("/list/:ino", func(c *gin.Context) {
        db, err = get_db(db_file)
        if err !=nil{
            fmt.Println("?? error opening database file:",db_file)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        defer db.Close()

        var pairs []string 
        if strings.Contains(c.Param("ino"),"&"){
            pairs =strings.Split(c.Param("ino"),"&")
        }else{
            pairs =append(pairs,c.Param("ino"))
        }
        dev_ino := pairs[0]
        dev_ino_pair := strings.Split(dev_ino,"_")
        device_id,err := strconv.ParseUint(dev_ino_pair[0],10,64)
        ino,err :=strconv.ParseUint(dev_ino_pair[1],10,64)
        var active_device_id uint64
        var active_ino uint64
        if len(pairs)>1{
            active_device_id,active_ino,err=dev_ino_uint64(pairs[1])
            if err !=nil{
                active_device_id=0
                active_ino=0
            }
        }
        
        url,err := file_url(db,device_id,ino,100,"/")
        all_nodes :=folder_entries(url)  
        is_root :=false
        if url==root_dir{
            is_root = true
        }
        refresh_folder(db,url,is_root)
        this_node,_:=query_fnode(db,device_id, ino)

        var folder_nodes []*Fnode
        var file_nodes []Fnode_view
        var shortcut_value string
        var shortcut_icon string

        notes_map,err:=get_note_map(db,device_id,ino,root_dir,db_folder)
        shortcut_map,err:=get_shortcut_map(db,url,root_dir)
        // this_folder :=path_file_name(relative_path_of(url,root_dir))
        _,ok :=shortcut_map["000root000"]
        if ok{
            shortcut_value="false"
            shortcut_icon="layui-icon-rate-solid"
        }else{
            shortcut_value="true"
            shortcut_icon="layui-icon-rate"
        }

        for _,tmp_node :=range(all_nodes){
            if tmp_node.IsDir {
                folder_nodes=append(folder_nodes,tmp_node)
            }else{
                fnv:=Fnode_to_view(tmp_node)
                record,ok := notes_map[tmp_node.Name]
                if ok{
                    fnv.Note =record.Note
                    fnv.Tag=record.Tag
                    fnv.Color=color_decode(record.Color)
                    fnv.Note_visible="note_visible"
                }else{
                    fnv.Note =""
                    fnv.Tag=""
                    fnv.Color=color_decode(0)
                    fnv.Note_visible=""
                }
                if uint64(fnv.Dev)==active_device_id && fnv.Ino == active_ino{
                    fnv.Active_css_class="active"
                }else{
                    fnv.Active_css_class=""
                }
                _,ok =shortcut_map[tmp_node.Name]
                if ok{
                    fnv.Pin_class="pinned"
                    fnv.Pin_value="false"
                }else{
                    fnv.Pin_class="unpinned"
                    fnv.Pin_value="true"
                }
                file_nodes = append(file_nodes,fnv)
            }
        }
        
        if err!=nil{
            // fmt.Printf("error:%#v",err)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        workspace_folders,err:=shortcut_entry(db,"d",root_dir)
        if err !=nil{
            workspace_folders=""
        }
        workspace_files,err:=shortcut_entry(db,"f",root_dir)
        if err !=nil{
            workspace_files=""
        }
        var folder_name_maxlen=50
        var file_name_maxlen=120
        for i:=0;i<len(folder_nodes);i++{
            if len(folder_nodes[i].Name) >folder_name_maxlen{
                folder_nodes[i].Name = str_shrink(folder_nodes[i].Name,folder_name_maxlen)
            }
        }

        for i:=0;i<len(file_nodes);i++{
            if len(file_nodes[i].Name)>file_name_maxlen{
                file_nodes[i].Name=str_shrink(file_nodes[i].Name,file_name_maxlen)
            }
        }

        c.HTML(http.StatusOK,"index.html",gin.H{
            "folder_nodes":folder_nodes,
            "file_nodes":file_nodes,
            "url":url,
            "dev_ino":dev_ino,
            "parent_dev_ino":dev_ino_pair[0]+"_"+strconv.FormatUint(this_node.Parent_ino,10),
            "shortcut_value":shortcut_value,
            "shortcut_icon":shortcut_icon,
            "workspace_folders":workspace_folders,
            "workspace_files":workspace_files,
        })
    })

    // ============= RESOURCE HANDLE =====================
    r.POST("/image_upload",func(c *gin.Context){
        db, err = get_db(db_file)
        if err !=nil{
            fmt.Println("?? error opening database file:",db_file)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        defer db.Close()
        
        file, _ := c.FormFile("file")
        // detail of the file
        // &multipart.FileHeader{
        // Filename:"aaa.png", 
        // Header:textproto.MIMEHeader{
        //   "Content-Disposition":[]string{"form-data; name=\"file\"; filename=\"aaa.png\""}, 
        //   "Content-Type":[]string{"image/png"}}, Size:9944, content
     
        content_type :=file.Header["Content-Type"][0]
        c.SaveUploadedFile(file, db_folder+"upload_temp")
        rs_type := mime_encode(content_type)
        tag,err:=resource_deposite_file(db,file.Filename,rs_type,db_folder+"upload_temp" ,db_folder)
        if err !=nil{
            c.JSON(http.StatusOK, gin.H{
                "location":"/get_image/xxx",
            })
        }else{
            c.JSON(http.StatusOK, gin.H{
                "location":"/get_image/"+tag,
            })
        }
    })
    r.POST("/image_update",func(c *gin.Context){
        db, err = get_db(db_file)
        if err !=nil{
            fmt.Println("?? error opening database file:",db_file)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        file, _ := c.FormFile("file")
        tag:=c.PostForm("tag")    
        content_type :=file.Header["Content-Type"][0]
        c.SaveUploadedFile(file, db_folder+"upload_temp")
        handler, err := os.Open( db_folder+"upload_temp")
        defer  handler.Close()
        if err!=nil{
            c.String(http.StatusOK,"??open file faild")
        }
        data,err:=ioutil.ReadAll(handler)
        if err !=nil{
            c.String(http.StatusOK,"??read file faild")
        }
        rs_type := mime_encode(content_type)
       
        _,err=resource_update(db,tag,rs_type,data,db_folder)
        if err!=nil{
            c.String(http.StatusOK,"??update faild")
        }
        
        c.String(http.StatusOK,"!!"+tag)       
    });

    r.GET("/get_image/:tag",func(c *gin.Context){
        db, err = get_db(db_file)
        if err !=nil{
            fmt.Println("?? error opening database file:",db_file)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        defer db.Close()
        
        tag:=c.Param("tag")
        _,data,err:=get_image(db, db_folder,tag)
        if err!=nil{

        }else{
            mime_str,_ := get_image_mime(db,tag)
            c.Data(http.StatusOK,mime_str,data)
        }
    });
    r.GET("/get_image_r/:tag",func(c *gin.Context){
        // for change image, after changing the image,
        // if src does not change, the image do not get updated
        db, err = get_db(db_file)
        if err !=nil{
            fmt.Println("?? error opening database file:",db_file)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        defer db.Close()
        
        tag:=c.Param("tag")
        _,data,err:=get_image(db, db_folder,tag)
        if err!=nil{

        }else{
            mime_str,_ := get_image_mime(db,tag)
            c.Data(http.StatusOK,mime_str,data)
        }
    });

    r.GET("/list_image/:page",func(c *gin.Context){
        db, err = get_db(db_file)
        if err !=nil{
            fmt.Println("?? error opening database file:",db_file)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        defer db.Close()
        
        page,_ :=strconv.Atoi(c.Param("page"))
        page_len := 20
        cnt,err := image_count(db)

        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        images :=list_images(db, page_len,page)
        var page_count int
        if int(cnt)%page_len==0{
            page_count = int(cnt)/page_len
        }else{
            page_count = int(cnt)/page_len+1
        }

        c.HTML(http.StatusOK,"images.html",gin.H{
            "images":images,
            "page_bar":draw_page_bar(page_count,page,"background-color:#1E9FFF","/list_image/"),
        })
    });

    r.POST("/image_cname",func(c *gin.Context){
        db, err = get_db(db_file)
        if err !=nil{
            fmt.Println("?? error opening database file:",db_file)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        defer db.Close()
        
        tag :=c.PostForm("tag")
        name :=c.PostForm("new_name")
        ok,err :=resource_update_name(db,tag,name)
        if err !=nil{
            c.String(http.StatusOK,"??Data base error")
        }
        if !ok{
            c.String(http.StatusOK,"??not Changed")
        }
        c.String(http.StatusOK,"!!"+tag)
    });

    r.GET("/track/:tag",func(c *gin.Context){
        db, err = get_db(db_file)
        if err !=nil{
            fmt.Println("?? error opening database file:",db_file)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        defer db.Close()
        
        tag :=c.Param("tag")
        app,app_tag,err := resource_link_read(db,tag)
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/2")
        }
        if app == 1{
            record,err:=get_note_by_tag(db,app_tag)
            if err !=nil{
                c.Redirect(http.StatusTemporaryRedirect,"/error/1")
            }
            record_rel_path:=""
            if sys_delim()=="\\"{
                record_rel_path =strings.ReplaceAll(record.File_dir+record.File_name,"/","\\")
            }else{
                record_rel_path=record.File_dir+record.File_name
            }
            fnode,err:=get_Fnode(root_dir+record_rel_path,false)
            if err !=nil{
                c.Redirect(http.StatusTemporaryRedirect,"/error/2")
            }
            active_ino :=strconv.FormatUint(uint64(fnode.Dev),10)+"_"+strconv.FormatUint(fnode.Ino,10)
            parent_ino :=strconv.FormatUint(uint64(fnode.Dev),10)+"_"+strconv.FormatUint(fnode.Parent_ino,10)
            c.Redirect(http.StatusTemporaryRedirect,"/list/"+parent_ino+"&"+active_ino)
        }else{
            c.Redirect(http.StatusTemporaryRedirect,"/error/2")
        }
    });
    //====================== NOTES HANDLE ======================
    r.POST("/add_note/:ino",func(c *gin.Context){
        // posting: 'ino_id' : ino_id,'tag': item_value, 'note':tinyMCE.get('note_content').getContent(),'color': color_code}
        db, err = get_db(db_file)
        if err !=nil{
            fmt.Println("?? error opening database file:",db_file)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        defer db.Close()
        
        ok,err:=regexp.MatchString(`\d+_\d+`,c.PostForm("ino_id"))

        if err!=nil{
            c.String(http.StatusOK,"?? query error")
        }
        if ok{
            pairs:=strings.Split(c.PostForm("ino_id"),"_")
            note:=c.PostForm("note")
            color:=c.PostForm("color")
            device_id := pairs[0]
            ino:=pairs[1]
            tag,err :=add_note(db,"virtual",device_id,ino,note,color,root_dir,db_folder)
            if err !=nil{
                c.String(http.StatusOK,"??error adding note-code1")
            }
            
            image_tags := extract_tags(note)
            for _,img_tag := range(image_tags){
                resource_ref_count_inc(db,img_tag)
                resource_link_add(db,img_tag,1,tag)
            }
            image_name_map := extract_img_names(note)
            for img_tag,name :=range image_name_map{
                resource_update_name(db,img_tag,name)
            }
            if err !=nil{
                c.String(http.StatusOK,"??error adding note-code2")
            }else{
                c.String(http.StatusOK,"!!"+tag)
            }
        }else{
            c.String(http.StatusOK,"??adding note failed-code4")
        }        

    });
    r.GET("/del_note/:ino",func(c *gin.Context){
        db, err = get_db(db_file)
        if err !=nil{
            fmt.Println("?? error opening database file:",db_file)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        defer db.Close()
        
        pair:=strings.Split(c.Param("ino"),"_")
        device_id,_:=strconv.ParseUint(pair[0],10,64)
        ino,_:=strconv.ParseUint(pair[1],10,64)
        url,err := file_url(db,device_id,ino,100,"/")
        if err !=nil{
            c.String(http.StatusOK,"??unable to find file")
        }
        rel_url := relative_path_of(url,root_dir)
        file_dir := path_dir_name(rel_url,"/")
        file_name := path_file_name(rel_url,"/")

        _,err = del_note(db, file_dir,file_name,db_folder)
        if err!=nil{
            c.String(http.StatusOK,"??del note failed")
        }else{
            c.String(http.StatusOK,"!!"+c.Param("ino"))
        }
    });

    r.POST("/edit_note/:tag",func(c *gin.Context){
       // posting {'ino_id' : ino_id,'tag': item_value, 'note':tinyMCE.get('note_content').getContent(),'color': color_code}
        db, err = get_db(db_file)
        if err !=nil{
            fmt.Println("?? error opening database file:",db_file)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        defer db.Close()
       
        tag :=c.PostForm("tag")
        note :=c.PostForm("note")
        color :=c.PostForm("color")
        ok,err:=edit_note(db,tag,note,color,db_folder)
        if err !=nil{
            c.String(http.StatusOK,"??update note error")
        }
        if !ok{
            c.String(http.StatusOK,"??update note failed")
        }
        c.String(http.StatusOK,"!!"+tag)
    });

    r.POST("/rename/:ino",func(c *gin.Context){
        db, err = get_db(db_file)
        if err !=nil{
            fmt.Println("?? error opening database file:",db_file)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        defer db.Close()
         
        dev_ino:=c.PostForm("ino_id")
        new_name :=c.PostForm("new_name")+"."+c.PostForm("new_name_ext")
        device_id,ino,err :=dev_ino_uint64(dev_ino)
        if err !=nil{
            fmt.Printf("error:%q\n",err)
            fmt.Println(dev_ino)
            c.String(http.StatusOK,"??Query error")
        }else{
            _,err=file_rename(db,device_id,ino,new_name ,root_dir )
            if err !=nil{
                fmt.Printf("error:%q\n",err)
                c.String(http.StatusOK,"??rename_error")
            }else{
                c.String(http.StatusOK,"!!done:oldname:"+new_name)
            }
        }
    });


    r.GET("/file_notes/:page",func(c *gin.Context){
        db, err = get_db(db_file)
        if err !=nil{
            fmt.Println("?? error opening database file:",db_file)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        defer db.Close()
        
        page,_ :=strconv.Atoi(c.Param("page"))
        page_len := 20
        cnt,err := notes_count(db)
        if err !=nil{
            c.HTML(http.StatusOK,"error.html",gin.H{
                "error_msg":"count image error",
            })
        }
        all_notes,err:=list_notes(db, page_len,page,db_folder)
        if err !=nil{
            c.HTML(http.StatusOK,"error.html",gin.H{
                "error_msg":"count image error",
            })
        }
        var page_count int
        if int(cnt)%page_len==0{
            page_count = int(cnt)/page_len
        }else{
            page_count = int(cnt)/page_len+1
        }
        c.HTML(http.StatusOK,"notes.html",gin.H{
            "notes":all_notes,
            "page_bar":draw_page_bar(page_count,page,"background-color:#1E9FFF","/file_notes/"),
        })

    });
    
    // ================= FILE OPEN =========================
    r.GET("/show/:dev_ino",func(c *gin.Context){  
        db, err = get_db(db_file)
        if err !=nil{
            fmt.Println("?? error opening database file:",db_file)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        defer db.Close()

        device_id,ino,err:=dev_ino_uint64(c.Param("dev_ino"))
        if err !=nil{
            c.String(http.StatusOK,"??query error")
        }
        url,err := file_url(db,device_id,ino,100,"/")
        if err !=nil{
            c.String(http.StatusOK,"??error")
        }
       
        file_ext :=file_suffix(url)
        switch file_ext{
        case "pdf","pptx","ppt","docx","doc":
            fnode,err :=query_fnode(db,device_id,ino)
            cmd := exec.Command("open",url)
            err = cmd.Start()
            if err !=nil{
                c.String(http.StatusOK,"??error")
            }
            folder_dev_ino := strconv.FormatUint(device_id,10)+"_"+strconv.FormatUint(fnode.Parent_ino,10)
            active_dev_ino := strconv.FormatUint(device_id,10)+"_"+strconv.FormatUint(ino,10)
            fmt.Println("redirect->"+"/list/"+strconv.FormatUint(device_id,10)+"_"+strconv.FormatUint(fnode.Parent_ino,10))
            // http.StatusMovedPermanently not suitable
            c.Redirect(http.StatusTemporaryRedirect,"/list/"+folder_dev_ino+"&"+active_dev_ino)
       
        case "rb","py","go","c","cpp","h","php","html","pl","cs","asp","erb":
            file_handler,err :=os.Open(url)
            defer file_handler.Close()
            if err!=nil{
                c.Redirect(http.StatusTemporaryRedirect,"/error/1")
            }
            data,err := ioutil.ReadAll(file_handler)
            if err!=nil{
                c.Redirect(http.StatusTemporaryRedirect,"/error/1")
            }

            c.HTML(http.StatusOK,"show_code.html",gin.H{
                "code_type":file_ext,
                "code_content":string(data),
            })
        case "png","jpeg","jpg","bmp","tif","svg","mp3","webp":
            file_handler,err :=os.Open(url)
            defer file_handler.Close()
            if err!=nil{
                c.Redirect(http.StatusTemporaryRedirect,"/error/1")
            }
            data,err := ioutil.ReadAll(file_handler)
            if err!=nil{
                c.Redirect(http.StatusTemporaryRedirect,"/error/1")
            }
            c.Data(http.StatusOK,ext_to_mime(file_ext),data)
        default:
            file_handler,err :=os.Open(url)
            defer file_handler.Close()
            if err!=nil{
                c.Redirect(http.StatusTemporaryRedirect,"/error/1")
            }
            data,err := ioutil.ReadAll(file_handler)
            if err!=nil{
                c.Redirect(http.StatusTemporaryRedirect,"/error/1")
            }
            c.Header("Content-Type", ext_to_mime(file_ext))
            c.Header("Content-Disposition","filename="+path_file_name(url,"/"))
            c.Data(http.StatusOK,ext_to_mime(file_ext),data)
        }

    });
   
    // ================= shortcut =======================
    r.GET("/add_shortcut/:ino",func(c *gin.Context){
        db, err = get_db(db_file)
        if err !=nil{
            fmt.Println("?? error opening database file:",db_file)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        defer db.Close()

        device_id,ino,err:=dev_ino_uint64(c.Param("ino"))
        if err!=nil{
            c.String(http.StatusOK,"??query error")
        }else{
            url,err := file_url(db,device_id,ino,100,"/")
            if err!=nil{
                c.String(http.StatusOK,"??db error")
            }
            
            rel_url := relative_path_of(url,root_dir)
            file_name:= path_file_name(rel_url,"/")
            file_dir:= path_dir_name(rel_url,"/")
            sc_type :="f"
            if strings.HasSuffix(url,"/"){
                sc_type="d"
            }
            ok,err:=add_shortcut(db,file_dir,file_name,sc_type)
            if err!=nil{
                c.String(http.StatusOK,"??not done")
            }else{
                if ok{
                    c.String(http.StatusOK,"!!done")
                }else{
                    c.String(http.StatusOK,"??existing")
                }
            }           
        }
    });

    r.GET("/del_shortcut/:ino",func(c *gin.Context){
        db, err = get_db(db_file)
        if err !=nil{
            fmt.Println("?? error opening database file:",db_file)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        defer db.Close()

        device_id,ino,err:=dev_ino_uint64(c.Param("ino"))
        if err!=nil{
            c.String(http.StatusOK,"??query error")
        }else{
            url,err :=file_url(db,device_id,ino,100,"/")
            if err!=nil{
                c.String(http.StatusOK,"??db error")
            }
            rel_url := relative_path_of(url,root_dir)
            file_name:= path_file_name(rel_url,"/")
            file_dir:= path_dir_name(rel_url,"/")
            ok,err:=del_shortcut(db,file_dir,file_name)
            if err!=nil{
                c.String(http.StatusOK,"??not done")
            }else{
                if ok{
                    c.String(http.StatusOK,"!!done")
                }else{
                    c.String(http.StatusOK,"??existing")
                }
            }           
        }
    });

    r.GET("/rebuild",func(c *gin.Context){
        db, err = get_db(db_file)
        if err !=nil{
            fmt.Println("?? error opening database file:",db_file)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        defer db.Close()
        
        if _,err:=clear_ino(db,root_dir);err !=nil{
            c.String(http.StatusOK,"??rebuild error")
        }else{
            if _, err:=rebuild(db, root_dir); err !=nil{
                c.String(http.StatusOK,"??rebuild error")
            }else{
                c.String(http.StatusOK,"!!Done")
            }
        }
    });

    r.GET("/settings",func(c *gin.Context){
        c.Redirect(http.StatusTemporaryRedirect,"/error/101")
    });

    r.GET("/manange_shortcut",func(c *gin.Context){
        c.Redirect(http.StatusTemporaryRedirect,"/error/101")
    });

    r.GET("/gallary",func(c *gin.Context){
        c.Redirect(http.StatusTemporaryRedirect,"/error/101")
    });

    r.GET("/notes_orphans",func(c *gin.Context){
        c.Redirect(http.StatusTemporaryRedirect,"/error/101")
    });
    r.GET("/error/:no",func(c *gin.Context){
        db, err = get_db(db_file)
        if err !=nil{
            fmt.Println("?? error opening database file:",db_file)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        defer db.Close()
        error_no:=c.Param("no")
        switch error_no{
        case "1":
            c.HTML(http.StatusOK,"error.html",gin.H{
                "error_msg":"program inner error",
            })
        case "2":
            c.HTML(http.StatusOK,"error.html",gin.H{
                "error_msg":"resource not found",
            })
        case "101":
            c.HTML(http.StatusOK,"error.html",gin.H{
                "error_msg":"To be implemented",
            })
        default :
            c.HTML(http.StatusOK,"error.html",gin.H{
                "error_msg":"program inner error",
            })
        }
    });

 
    

    var server_ip string
    if *expose_server{
        server_ip ="0.0.0.0"
    }else{
        server_ip ="127.0.0.1"
    }
    r.Run(server_ip+":"+strconv.Itoa(*app_port)) //默认在本地8080端口启动服务
}