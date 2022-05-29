// Author: Hengyi Jiang <hengyi.jiang@gmail.com>
// 2022-05-29
// Version 0.2
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
    // "golang.org/x/sys/windows"
    "strings"
    "strconv"
    "errors"
    "github.com/gin-gonic/gin"
    "net/http"
    "bytes"
    "crypto/rand"
    "time"
    "regexp"
    "html/template"
    "path/filepath"
    "flag"
    "sort"
    "runtime"
)

// Basic types
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
    Stash_class string
    Stash_value string
}

// for *[]Fnode sort
type byAlpha []*Fnode
func(x byAlpha) Len() int {return len(x)}
func(x byAlpha) Less(i,j int) bool {return strings.ToLower(x[i].Name)<strings.ToLower(x[j].Name) }
func(x byAlpha) Swap(i,j int) {x[i],x[j]=x[j],x[i]}

// for Note_record
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

type Resource_record struct{
    Tag string
    Name string
    Rs_type int
    Page int
    Rs_date string
    Ref_count int
    File_name string
}


// for SQL packing
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
    }else{
        panic("table column "+name+" not defined")
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
        if strings.Contains(value,"%"){
            where_arr = append(where_arr, col+" like "+value)
        }else if strings.HasPrefix(value,">") || strings.HasPrefix(value,"<"){
            where_arr = append(where_arr,col+value)
        }else{
            where_arr = append(where_arr,col+"="+value)
        }  
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
    defer rows.Close()
    if err !=nil{
        return 0,err
    }
    rows.Next()
    var count  int64
    err = rows.Scan(&count)
   
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

// for tables
func get_table(tab_name string) *Db_table{
    tab:=new(Db_table)
    switch tab_name{
    case "article":
        tab.set_name("article").add_column("artid",false).add_column("tag",true)
        tab.add_column("title",true).add_column("color",false).add_column("shelf_id",false).add_column("adate",true)
    case "article_page":
        tab.set_name("article_page").add_column("pgid",false).add_column("pg_tag",true).add_column("tag",true)
        tab.add_column("pdate",true).add_column("order_id",false)
    case "file_note":
        tab.set_name("file_note")
        tab.add_column("tag",true).add_column("file_dir",true).add_column("file_name",true).add_column("tag",true)
        tab.add_column("note",true).add_column("color",false).add_column("ndate",true).add_column("nid",false)
    case "ino_tree":
        tab.set_name("ino_tree").add_column("id",false).add_column("host_name",true)
        tab.add_column("device_id",false).add_column("ino",false).add_column("parent_ino",false)
        tab.add_column("name",true).add_column("type",true).add_column("state",true)
    case "resource":
        tab.set_name("resource").add_column("rsid",false)
        tab.add_column("tag",true).add_column("page",false).add_column("name",true)
        tab.add_column("type",false).add_column("rs_date",true).add_column("ref_count",false)
    case "resource_link":
        tab.set_name("resource_link").add_column("tag",true).add_column("app",false).add_column("app_tag",true)
    case "settings":
        tab.set_name("settings").add_column("id",false).add_column("key",true).add_column("value",true).add_column("note",true)
    case "shortcut":
        tab.set_name("shortcut").add_column("file_dir",true).add_column("file_name",true).add_column("type",true)
        tab.add_column("track_id",false).add_column("order_id",false).add_column("scid",false)
    case "tags":
        tab.set_name("tags").add_column("tag_str",true)
    default:
        fmt.Println("!!warning table name not in the set")
    }
    return tab
}


// for fs handling
func sys_delim()string{
    return string(os.PathSeparator)
}

func str_native_delim(input string)string{
    delim :=sys_delim()
    if strings.Contains(input,"/") && delim =="\\"{
        return strings.ReplaceAll(input,"/","\\")
    }
    return input    
}

func str_db_delim(input string)string{
    if strings.Contains(input,"\\"){
        return strings.ReplaceAll(input,"\\","/")
    }
    return input
}


// windows specific codes -------------------------------------------------------------
// for fs handling
// func win_dev_ino(fname string,is_dir bool)(int32,uint64, error) {
//     _fname, err := windows.UTF16PtrFromString(fname)
//     if err != nil {
//         return 0, 0, err
//     }
//     var handle windows.Handle
//     if is_dir{
//         handle, err = windows.CreateFile(_fname,windows.GENERIC_READ,windows.FILE_SHARE_READ,nil,windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
//         if err != nil {
//             return 0,0, err
//         }
//     }else{
//         handle, err = windows.CreateFile(_fname,windows.GENERIC_READ,windows.FILE_SHARE_READ,nil,windows.OPEN_EXISTING, 0 , 0)
//         if err != nil {
//             return 0,0, err
//         }
//     }

//     defer windows.CloseHandle(handle)
//     var data windows.ByHandleFileInformation
//     if err = windows.GetFileInformationByHandle(handle, &data); err != nil {
//         return 0, 0, err
//     }
//     return int32(data.VolumeSerialNumber), (uint64(data.FileIndexHigh) << 32) | uint64(data.FileIndexLow), nil
// }
// }

// func folder_entries(path string) []*Fnode{
//     var values  []*Fnode
//     // pnt_fileinfo, _ := os.Stat(path)
//     // pnt_stat, ok := pnt_fileinfo.Sys().(*syscall.Stat_t)

//     device_id,pnt_fileid,err := win_dev_ino(path,true)
//     if err !=nil{
//         return values
//     }
//     if err!=nil {
//         return values //empty
//     }
//     fileInfos,err := ioutil.ReadDir(path)
//     if err !=nil{return values}
//     for _,info :=range fileInfos{
//         // stat, ok := info.Sys().(*syscall.Stat_t)
//         _,file_id,err := win_dev_ino(path+info.Name(),info.IsDir())
//         if err ==nil{
//             if ! strings.HasPrefix(info.Name(),"."){
//                 // values=append(values,&Fnode{info.Name(),info.IsDir(),stat.Dev,stat.Ino,pnt_stat.Dev,pnt_stat.Ino})
//                 values=append(values,&Fnode{info.Name(),info.IsDir(),device_id,file_id,device_id,pnt_fileid})
//             }
//         }        
//     }
//     return values
// }


// func get_Fnode(path string,is_root bool) (*Fnode,error){
//     var result Fnode
//     delim :=sys_delim()
//     if delim=="\\"{
//         path = strings.ReplaceAll(path, "/","\\")
//     }
//     info, err := os.Stat(path)
//     if err!=nil{
//         return &result,err
//     }
   
//     if err!=nil{
//         return &result,err
//     }
//     result.IsDir=info.IsDir()
//     result.Dev,result.Ino,err = win_dev_ino(path,info.IsDir())
//     if err!=nil{
//         return &result,err
//     }
//     if is_root{
//         ensure_folder(&path,delim)
//         if strings.HasSuffix(path,delim){
//             result.Name=path[0:(len(path)-1)]
//         }else{
//             result.Name=path
//         }        
//         result.Parent_dev=result.Dev
//         result.Parent_ino=result.Ino
//     }else{
//         result.Name=info.Name()
//         var tmp_path string
//         if strings.HasSuffix(path,delim){
//             tmp_path =path_dir_name(path[0:(len(path)-1)],delim)
//         }else{
//             tmp_path =path_dir_name(path[0:(len(path))],delim)
//         }
         
//         result.Parent_dev,result.Parent_ino, err = win_dev_ino(tmp_path,true)
//         if err!=nil{
//             return &result,err
//         }
//     }
//     return &result,nil
// }

//---------------------- end of windows specific code-----------------------------------
//++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++

//  mac specific codes
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
// end of mac specific codes++++++++++++++++++++++++++++++++++++++++++++++++++

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


//====================================================================================================
// for ino_tree

func register_ino(db_link *sql.DB,node *Fnode)(bool,error){
// do query first
    table:=get_table("ino_tree")    
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
    table:=get_table("ino_tree")    
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
    table:=get_table("ino_tree")    
    host_name :=get_host_name()
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
    table:=get_table("ino_tree")
    table.set("device_id",strconv.FormatUint(uint64(fnode.Dev),10)).set("host_name",host_name)
    _,err =do_delete(db_link,table.pack_delete())
    if err !=nil{
        return false,err
    }
    return true,nil
}

func query_fnode(db_link *sql.DB,device_id uint64, ino uint64) (*Fnode,error){
    table:=get_table("ino_tree")
    host_name:=get_host_name()
    table.set("device_id",strconv.FormatUint(device_id,10)).set("ino",strconv.FormatUint(ino,10)).set("host_name",host_name)
    var node Fnode
    tp :="f"
    rows, err := db_link.Query(table.pack_select("device_id,ino,parent_ino,name,type","name asc","1"))
    defer rows.Close()
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
    
    if i==0{
        return &node,errors.New("no result")
    }
    return &node,nil
}

func search_fnodes(db_link *sql.DB,host_name string,name string)([]Fnode,error){
    var node Fnode
    var result []Fnode
    table := get_table("ino_tree")
    table.set("host_name",host_name).set("name","%"+name+"%")
    rows, err := db_link.Query(table.pack_select("device_id,ino,parent_ino,name,type","name asc",""))
    defer rows.Close()
    if err !=nil{
        return result,err
    }
    tp :="f"
    for rows.Next(){        
        err = rows.Scan(&node.Dev,&node.Ino,&node.Parent_ino,&node.Name,&tp)
        if err ==nil{
            node.IsDir=false
            if tp=="d"{
                node.IsDir=true
            }
            result=append(result,node)
        }        
    }
    return result,err  
}

func file_url(db_link *sql.DB,device_id uint64, ino uint64,max_level int,delim string) (string,error){
    node, err := query_fnode(db_link,device_id,ino)
    if max_level<0{
        // guard against infinite loop error
        return "",errors.New("maxium iteration")
    }
    if err !=nil{
        return "",errors.New("query fnode error")
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
        return "",errors.New("iteration error")
    }
    // fmt.Printf("%s=>%s",node.Name,s)
    return parent+node.Name+s,nil
}



// set calculation
func inos_in_parent(db_link *sql.DB,device_id uint64,parent_id uint64)([]int64){
    table:=get_table("ino_tree")
    host_name:=get_host_name()
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
            _,err=delete_ino(db_link,&temp_fnode)
            if err !=nil{
                fmt.Printf("!!error:%s\n",err.Error())
            }
           
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

func file_no_repeat(path string)(string,error){
    file_dir :=path_dir_name(path,sys_delim())
    file_name :=path_file_name(path,sys_delim())
    if ok,_:=file_exists(path);ok{
        var base_name string
        var ext string
        var result string
        if strings.Contains(file_name,"."){
            idx := strings.LastIndex(path,".")
            base_name =path[0:idx]
            ext =path[idx:len(path)]
            result=base_name+"(1)"+ext
        }else{
            base_name =path
            ext =""
            result=path+"(1)"
        }
        if ok,_:=file_exists(result);!ok{
            return result,nil
        }

        // this is already filename(1)(2).xxx in the folder
        // then scan the folder
        reg:=regexp.MustCompile(`^(.*)\((\d+)\)\.([\d\w_]*)$`)
        if !strings.Contains(file_name,"."){
            reg=regexp.MustCompile(`^(.*)\((\d+)\)$`)
        }
        entries,err :=ioutil.ReadDir(file_dir)
        if err!=nil{
            return "",err
        }
        order_max :=0
        for _,info :=range(entries){
            if reg.MatchString(info.Name()){
                mats := reg.FindStringSubmatch(info.Name())
                if len(mats)>1{
                    order_number,err :=strconv.Atoi(mats[2])
                    if err!=nil{
                        return "",err
                    }
                    if order_number > order_max{
                        order_max =order_number
                    }
                }    
            }
        }
        order_max +=1
        return base_name+"("+strconv.Itoa(order_max)+")"+ext,nil
    }
    return path,nil
}

func file_safe_mv(old_path string,new_path string,force bool)(string,error){
    var new_target string
	if ok,err:=file_exists(new_path);ok{
        if force{
            new_target,err =file_no_repeat(new_path)
            if err !=nil{
                return "",err
            }
        }else{
            return "",errors.New("file already exists")
        }
    }else{
        new_target=new_path
    }
    err:=os.Rename(old_path,new_target)
    if err!=nil{
        return "", err
    }
    return new_target,nil
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
        1: "image/png",
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

func mime_decode_suffix(rs_type int )string{
    str:=mime_decode(rs_type)
    r := strings.Split(str,"/")
    if r[1]=="svg+xml"{
        return "svg"
    }
    return r[1]
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
    case "pdf":
        r="application/pdf"
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

func str_shrink_rune(input string,max_len int)string{
    input_r :=[]rune(input)
    if len(input)<max_len{
        return input
    }
  
    if len(input_r)>max_len{
		return string(input_r[0:(max_len)])
	}
	return string(input_r)
}

func html_shrink(input string,max_len int)string{
    reg := regexp.MustCompile(`<h\d>(.*?)</h\d>?`)
	mats:=reg.FindString(input)
	if mats !=""{
		return mats
	}
	reg = regexp.MustCompile(`<\/?\w+.*?>`)
	text :=reg.ReplaceAllString(input, "")
	return str_shrink_rune(text,max_len)
}

// SET OPERATION
type Set struct{
    keys map[string]bool
	count int
}

func make_set(array []string) *Set{   
	m:=make(map[string]bool)
	cnt :=0
    for _,k:=range(array){
        _,ok := m[k]
        if !ok{
            m[k]=true
			cnt++
        }
    }
	return &Set{keys:m,count:cnt}
}

func (s *Set) Has(k string) bool{
	_,ok := s.keys[k]
	return ok
}

func (s *Set) Add_array(a []string) *Set{
	for _,k:=range(a){
        if ! s.Has(k){
			s.Add(k)
		}
    }
	return s
}

func (s *Set) Add(k string) *Set{
	if !s.Has(k){
		s.keys[k]=true
		s.count++
	}
	return s
}

func (s *Set) Del(k string) *Set{
	if s.Has(k){
		delete(s.keys,k)
		s.count--
	}
	return s
}

func (s *Set) List() []string{
	var v []string
	for k,_:=range(s.keys){
		v = append(v,k)
	}
	return v
}

func (s *Set) Substract_list(list []string) *Set{
	for _,l:=range(list){
		s.Del(l)
	}
	return s
}

func (s *Set) Substract(t *Set) *Set{
	l:=t.List()
	for _,ll :=range(l){
		if s.Has(ll){
			s.Del(ll)
		}
	}
	return s
}


func (s *Set) Union(t *Set) *Set{
	r :=make_set([]string{})

	for k,_ :=range(s.keys){
		if !r.Has(k){
			r.Add(k)
		}
	}
	for k,_ :=range(t.keys){
		if !r.Has(k){
			r.Add(k)
		}
	}
	return r
}


func  (s *Set) Intersect(t *Set) *Set{
	r :=make_set([]string{})

	for k,_ :=range(s.keys){
		if t.Has(k){
			r.Add(k)
		}
	}
	return r
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
        fmt.Println("?? error when openning db file: "+db_file)
        return db,err
    }
    return db,nil
}

//====================================================================================================
// for blob
func random_str(n int) string{
    var buf bytes.Buffer
    b := make([]byte, n)
    rand.Read(b)
    str :="abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPSTUVWXYZ1234567890"
    for _,i:=range(b){
        buf.WriteByte(str[int(i)%(len(str))])
    }
    return buf.String()
}

func tag_exist(db_link *sql.DB,tag string)(bool,error){
    table := get_table("tags")
    table.set("tag_str",tag)
    cnt,err:=do_count(db_link,table.pack_count("cnt"))
    if err !=nil{
        return false,err
    }
    if cnt==0{
        return false,nil
    }
    return true,nil
}

func tag_gen(db_link *sql.DB)(string,error){
    var tag string
    for{
        tag =random_str(10)
        ok,_:=tag_exist(db_link,tag)
        if ok{
            continue
        }
        break
    }
    
    table := get_table("tags")
    table.set("tag_str",tag)
    _,err := do_insert(db_link,table.pack_insert())
    if err !=nil{
        return tag,err
    }
    return tag,nil
}

func tag_clear(db_link *sql.DB,tag string)(bool,error){
    table := get_table("tags")
    table.set("tag_str",tag)
    _,err :=do_delete(db_link,table.pack_delete())
    if err !=nil{
        return false,err
    }
    return true,nil
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
   
    _, err = db.Exec(sql_str)
    defer db.Close()
    if err != nil {
        fmt.Printf("?? error creating blob_obj table:\n%s\n",sql_str)
        return false,err
    }
    return true,nil
}

func blob_save(db_file string,tag string,bin_data []byte,file_type int)(string,error){
    db, err := sql.Open("sqlite3",db_file)
    defer db.Close()
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
    return tag,nil
}

func blob_update(db_file string,tag string,bin_data []byte,file_type int)(bool,error){
    db, err := sql.Open("sqlite3",db_file)
    defer db.Close()
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
    return true,nil
}


func blob_read(db_file string,tag string)(int,[]byte,error){
    db,err :=sql.Open("sqlite3",db_file)
    defer db.Close()
    sql_str :="select type,data from blob_obj where tag=\""+tag+"\""
    rows,err :=db.Query(sql_str)
    defer rows.Close()
    if err !=nil{
        return 0,[]byte{},err
    }
    rows.Next()
    var rs_type int
    var data []byte
    err = rows.Scan(&rs_type,&data)
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
    defer db.Close()
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
    return true,nil
}

func blob_search(db_file string,target string)([]string,error){
    db,err :=sql.Open("sqlite3",db_file)
    var result []string
    defer db.Close()
    target=strings.ReplaceAll(target,"\"","\"\"")
    sql_search := "select tag from blob_obj where type>30 and data like \"%"+target+"%\""
    rs,err := db.Query(sql_search)
    defer rs.Close()
    if err !=nil{
        fmt.Printf("?? error doing blob_obj search:%s\n",err.Error())
        fmt.Println(sql_search)
        return result,err
    }
	var tag string
    for rs.Next(){
		rs.Scan(&tag)
		result=append(result, tag)
    }
	return result,nil
}

// ====================================================================================================
// for resource

func resource_deposite(db_link *sql.DB,name string,rs_type int,data []byte,db_folder string)(string,error){
    tab_resource:=get_table("resource")
    page,err:=get_blob_file_page(db_folder,50000000) // use constant later on
    if err !=nil{
        fmt.Println("?? get page failed")
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
        fmt.Println("?? tag generation failed")
        return "",err
    }

    _,err=blob_save(blob_file,tag,data,rs_type)
    if err !=nil{
        // delete the tag first [missing here]
        // then return
        fmt.Println("?? Blob save failed")
        return "",err
    }

    tab_resource.set("tag",tag).set("page",page).set("name",name).set("type",strconv.Itoa(rs_type))
    tab_resource.set("ref_count","0").set("rs_date",get_now_string())

    _,err=do_insert(db_link,tab_resource.pack_insert())
    if err !=nil{
        fmt.Println("?? Resource table save failed")
        return "",err
    }
    return tag,nil
}

func resource_deposite_file(db_link *sql.DB,name string,rs_type int,file_name string,db_folder string)(string,error){
    handler, err := os.Open(file_name)
    defer  handler.Close()
    if err!=nil{
        fmt.Println("?? open file failed")
        return "",err
    }
    data,err:=ioutil.ReadAll(handler)
    if err !=nil{
        fmt.Println("?? read file failed")
        return "",err
    }
    tag,err :=resource_deposite(db_link,name,rs_type,data,db_folder)
    return tag,err
}

func resource_ref_count_inc(db_link *sql.DB,tag string)(bool,error){
    tab_resource:=get_table("resource")
    tab_resource.set("ref_count","ref_count+1").set("tag",tag)
    check:=[]string{"tag"}
    _,err := do_update(db_link,tab_resource.pack_update(check))
    if err !=nil{
        fmt.Println("?? resource ref_count_inc failed")
        return false,err
    }
    return true,nil
}

func resource_ref_count_dec(db_link *sql.DB,tag string)(bool,error){
    tab_resource:=get_table("resource")
    tab_resource.set("ref_count","ref_count-1").set("tag",tag)
    check:=[]string{"tag"}
    _,err := do_update(db_link,tab_resource.pack_update(check))
    if err !=nil{
        fmt.Println("?? resource ref_count_dec failed")
        return false,err
    }
    return true,nil
}

func resource_update_name(db_link *sql.DB,tag string,name string)(bool,error){
    tab_resource:=get_table("resource")
    tab_resource.set("name",name).set("tag",tag)
    check:=[]string{"tag"}
    _,err := do_update(db_link,tab_resource.pack_update(check))
    if err !=nil{
        fmt.Println("?? resource resource_update_name failed")
        return false,err
    }
    return true,nil
}

func resource_update_type(db_link *sql.DB,tag string,rs_type int)(bool,error){
    tab_resource:=get_table("resource")
    tab_resource.set("type",strconv.Itoa(rs_type)).set("tag",tag)
    check:=[]string{"tag"}
    _,err := do_update(db_link,tab_resource.pack_update(check))
    if err !=nil{
        fmt.Println("?? resource resource_update_type failed"+err.Error())
        return false,err
    }
    return true,nil
}

func get_resource_record(db_link *sql.DB,tag string)(Resource_record,error) {
    tab_resource:=get_table("resource")
    tab_resource.set("tag",tag)
    var record Resource_record
    cnt,err :=do_count(db_link,tab_resource.pack_count("cnt"))
    if err !=nil{
        fmt.Println("?? resource get count failed")
        return record,err
    }
    if cnt <1{
        return record,errors.New("no record")
    }
    rows,err:=db_link.Query(tab_resource.pack_select("name,page,type,rs_date,ref_count","",""))
    defer rows.Close()
    if err !=nil{
        fmt.Println("?? resource get row failed")
        return record,err
    }
    if rows.Next(){
        err=rows.Scan(&record.Name, &record.Page, &record.Rs_type, &record.Rs_date,&record.Ref_count)
        record.Tag = tag
    
        if err !=nil{
            fmt.Println("?? resource scan row failed")
            return record,err
        }
        return record,nil
    }
    return record,errors.New("no record")
}


func get_image(db_link *sql.DB, db_folder string,tag string)(int,[]byte,error){
    tab_resource:=get_table("resource")
    tab_resource.set("tag",tag)
    page,err:=do_select_id(db_link,tab_resource.pack_select("page","",""))
    if err !=nil{
        fmt.Printf("sql:%s,error:%#v",tab_resource.pack_select("page","",""),err)
        return 0,[]byte{},err
    }
    if len(page)==0{
        return 0,[]byte{},errors.New("no record")
    }
    blob_file := db_folder +"blob"+strconv.FormatInt(page[0],10)+".db"
    rs_type,data,err :=blob_read(blob_file,tag)
    if err !=nil{
        return 0,[]byte{},err
    }
    return rs_type,data,nil
}

func get_image_mime(db_link *sql.DB, tag string)(string,error){
    tab_resource:=get_table("resource")
    tab_resource.set("tag",tag)
    rs_type,err:=do_select_id(db_link,tab_resource.pack_select("type","",""))
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

    tab_resource:=get_table("resource")
    tab_resource.add_column("type<",false)

    tab_resource.set("type<","10")
    rows,err :=db_link.Query(tab_resource.pack_select("tag,name,type,rs_date,ref_count","rsid desc",strconv.Itoa(start)+","+strconv.Itoa(page_len)))
    defer rows.Close();
    if err !=nil{
        return result
    }
    for rows.Next(){
        var item Resource_record
        rows.Scan(&item.Tag,&item.Name,&item.Rs_type,&item.Rs_date,&item.Ref_count)
        img_suffix := mime_decode_suffix(item.Rs_type)
        item.File_name=item.Tag+"."+img_suffix
        result = append(result,item)
    }
    return result
}

func search_images(db_link *sql.DB, target string) ([]Resource_record,error){
    var result =  []Resource_record{}
    sql_str:="select tag,name,type,rs_date,ref_count from resource where type<10 and name like \"%"+target+"%\" order by rsid desc"
    rows,err :=db_link.Query(sql_str)
    defer rows.Close();
    if err !=nil{
        return result,err
    }
    var item Resource_record
    for rows.Next(){
        rows.Scan(&item.Tag,&item.Name,&item.Rs_type,&item.Rs_date,&item.Ref_count)
        img_suffix := mime_decode_suffix(item.Rs_type)
        item.File_name=item.Tag+"."+img_suffix
        result = append(result,item)
    }
    return result,nil
}

func orphan_images(db_link *sql.DB) []Resource_record{
    var result =  []Resource_record{}
    tab_resource :=new(Db_table)
    tab_resource.set_name("resource")
    tab_resource.add_column("tag",true).add_column("type<",false).add_column("name",true).add_column("ref_count<",false)
    tab_resource.set("type<","10").set("ref_count<","0")
    rows,err :=db_link.Query(tab_resource.pack_select("tag,name,type,rs_date,ref_count","rsid desc",""))
    defer rows.Close()
    if err !=nil{
        return result
    }
    for rows.Next(){
        var item Resource_record
        rows.Scan(&item.Tag,&item.Name,&item.Rs_type,&item.Rs_date,&item.Ref_count)
        img_suffix := mime_decode_suffix(item.Rs_type)
        item.File_name=item.Tag+"."+img_suffix
        result = append(result,item)
    }
    return result
}

func img_name_tag(name string)string{
    if strings.Contains(name,"."){
        s:=strings.Split(name,".")
        return s[0]
    }
    return name
}

func extract_tags(text string)[]string{
    if !strings.Contains(text,`<img `){
        return []string{}
    }
    var result []string
    img_reg := regexp.MustCompile(`<img (.*?)>`)
	img_mats :=img_reg.FindAllStringSubmatch(text,-1)
	if len(img_mats)==0{
		return []string{} //empty
	}
	reg :=regexp.MustCompile(`src="\.?\.?/?get_image/([\w\d\.]+)"`)

    for _,r := range img_mats{
		mats:=reg.FindStringSubmatch(r[1])
		if len(mats)==0{
			continue
		}
        t :=img_name_tag(mats[1])
        result = append(result,t)
    }
    return result
}

func extract_img_names(text string)map[string]string{
    var result =make(map[string]string)
	if !strings.Contains(text,`<img `){
        return result
    }
   
    img_reg := regexp.MustCompile(`<img (.*?)>`)
	img_mats :=img_reg.FindAllStringSubmatch(text,-1)
	if len(img_mats)==0{
		return result
	}
	reg_src :=regexp.MustCompile(`src="\.?\.?/?get_image/([\w\d\.]+)"`)
	reg_alt :=regexp.MustCompile(`alt="(.*?)"`)

    for _,props := range img_mats{
		var tag, name string
		mats_src:=reg_src.FindStringSubmatch(props[1])
		if len(mats_src)==0{
			continue
		}
		tag = img_name_tag(mats_src[1])
		mats_alt:=reg_alt.FindStringSubmatch(props[1])
		name=""
		if len(mats_alt)!=0{
			name=mats_alt[1]
		}
		result[tag] = name
    }    
    return result
}

func resource_delete(db_link *sql.DB,tag string,db_folder string)(bool,error){
    tab_resource:=get_table("resource")
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
    tag_clear(db_link,tag)
    return true,nil
}


func resource_update(db_link *sql.DB,tag string,rs_type int,data []byte,db_folder string)(bool,error){
    tab_resource:=get_table("resource")
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

func resource_search(db_folder,target string,pages string)([]string,error){
    var result []string
    page_max,err := strconv.Atoi(pages)
    if err!=nil{
        return result,err
    }
    var tags []string
    for i:=1;i<(page_max+1);i++{
        blob_file := db_folder+"blob"+strconv.Itoa(i)+".db"
        tags,err=blob_search(blob_file,target)
        if err !=nil{
            return result,err
        }
        if len(tags)>0{
            for _,tag :=range(tags){
                result = append(result,tag)
            }
        }
    }
    return result,nil
}

func resource_ref_update_by_text(db_link *sql.DB,app int,app_tag string,old_note string,new_note string){
    old_tags := extract_tags(old_note)
    new_tags := extract_tags(new_note)
    new_tags_map :=extract_img_names(new_note)
    tags_to_update:=set_intersect(old_tags,new_tags)
    tags_to_delete:=set_substract(old_tags,tags_to_update)
    tags_to_insert:=set_substract(new_tags,tags_to_update)

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
        resource_link_add(db_link,item,app,app_tag)
    }

    //only decrease the ref_count, break the resource_link
    for _,item :=range(tags_to_delete){
        resource_ref_count_dec(db_link,item)
        resource_link_del(db_link,item,app,app_tag)
    }
}

func resource_ref_dec_by_text(db_link *sql.DB,app int,app_tag string,note_text string){
    res_tags := extract_tags(note_text)
    for _,tag := range(res_tags){
        resource_ref_count_dec(db_link,tag)
        resource_link_del(db_link,tag,app,app_tag)
    }
}

// resource link======================================================================================
func resource_link_add(db_link *sql.DB,tag string, app int, app_tag string)(bool,error){
    tab := get_table("resource_link")
    tab.set("tag",tag).set("app",strconv.Itoa(app)).set("app_tag",app_tag)
    _,err :=do_insert(db_link,tab.pack_insert())
    if err !=nil{
        return false,err
    }
    return true,nil
}

func resource_link_del(db_link *sql.DB,tag string, app int, app_tag string)(bool,error){
    tab := get_table("resource_link")
    tab.set("tag",tag).set("app",strconv.Itoa(app)).set("app_tag",app_tag)
    _,err :=do_insert(db_link,tab.pack_delete())
    if err !=nil{
        return false,err
    }
    return true,nil
}

func resource_link_read(db_link *sql.DB,tag string)(int, string, error){
    tab := get_table("resource_link")
    tab.set("tag",tag)
    var app int
    var app_tag string
    rows,err:=db_link.Query(tab.pack_select("app,app_tag","",""))
    defer rows.Close()
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
    tab_note:=get_table("file_note")
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
    file_name := path_file_name(relative_url,"/")
    file_dir := path_dir_name(relative_url,"/")
    tag,err := tag_gen(db_link)
    if err !=nil{
        // delete the tag
        return tag,err
    }

    // change from: blob_tag,err:=resource_deposite(db_link,"0x_text_"+get_now_string(),33,[]byte(note),db_folder)
    // the `name` field in resource table is now app tag
    // blob_tag is `tag` field in the resource table
    blob_tag,err:=resource_deposite(db_link,tag,33,[]byte(note),db_folder)
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
    resource_link_add(db_link,blob_tag,1,tag) // for note search
    
    // other resource_ref_count_inc in the note
    image_tags := extract_tags(note)
    // fmt.Printf("tags:%q",image_tags)
    for _,img_tag := range(image_tags){
        resource_ref_count_inc(db_link,img_tag)
        resource_link_add(db_link,img_tag,1,tag)
    }
    image_name_map := extract_img_names(note)
    for img_tag,name :=range image_name_map{
        resource_update_name(db_link,img_tag,name)
    }
    return tag,nil    
}

func get_note_record(db_link *sql.DB,file_dir string, file_name string) (Note_record,error){
    var result Note_record
    tab_note:=get_table("file_note")
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
    tab_note:=get_table("file_note")
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
    tab_note:=get_table("file_note")
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

    tab_note:=get_table("file_note")
    rows,err :=db_link.Query(tab_note.pack_select("tag,file_dir,file_name,note,ndate,color","nid desc",strconv.Itoa(start)+","+strconv.Itoa(page_len)))
    defer rows.Close()
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

func orphan_notes(db_link *sql.DB,db_folder string,root_dir string)([]Note_record,error){
    var result =  []Note_record{}
    page_len :=100000 //max
    page :=1
    all,err := list_notes_record(db_link,page_len,page)
    if err!=nil{
        return result, err
    }
    reg :=regexp.MustCompile(`#<0x_([\d\w]+)_>`)
    for i:=0;i<len(all);i++{
        row :=all[i]
        path :=root_dir + row.File_dir+row.File_name
        ok,_:=file_exists(path)
        if !ok{
            row.Color_str=color_decode(row.Color)
            mats :=reg.FindStringSubmatch(row.Note)
            if len(mats)>1{
                text_tag := mats[1]
                _,real_note,err :=get_text(db_link,db_folder,text_tag)
                if err ==nil{
                    row.Note = real_note
                }
            }
            result = append(result,row)
        }  
    }
    return result,nil
}

func search_notes(db_link *sql.DB,db_folder string,target string)([]Note_record,error){
    var result []Note_record
    max_pages,err := get_blob_file_page(db_folder,50000000) // use constant later on
    if err !=nil{
        return result, err
    }
    all_tags,err := resource_search(db_folder,target,max_pages)
    if err!=nil{
        return result,err
    }
    if len(all_tags)==0{
        return result,errors.New("no record")        
    }
    sql:="select app_tag from resource_link where tag in(\""+strings.Join(all_tags,"\",\"")+"\")"
    res,err :=db_link.Query(sql)
    defer res.Close()
    if err !=nil{
        return result,err
    }
    var app_tag string
    var app_tags []string
    for res.Next(){
        res.Scan(&app_tag)
        app_tags = append(app_tags,app_tag)
    }
    if len(app_tags)==0{
        return result,errors.New("no record") 
    }
    sql ="SELECT tag,file_dir,file_name,note,ndate,color from file_note where tag in(\""+strings.Join(app_tags,"\",\"")+"\") order by nid desc"
    rows,err :=db_link.Query(sql)
    defer rows.Close()
    if err !=nil{
        return result,err
    }
    var row Note_record
    reg :=regexp.MustCompile(`#<0x_([\d\w]+)_>`)
    for rows.Next(){
        rows.Scan(&row.Tag,&row.File_dir,&row.File_name,&row.Note,&row.Ndate,&row.Color)
        row.Color_str=color_decode(row.Color)
        mats :=reg.FindStringSubmatch(row.Note)
        if len(mats)>1{
            text_tag := mats[1]
            _,real_note,err :=get_text(db_link,db_folder,text_tag)
            if err ==nil{
                row.Note = real_note
            }
        }
        result =append(result,row)
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
    var text_tag string
    // there are others to delete:
    // 1. the resource related to this note
    // 2. the blob
    if len(mats)>1{
        text_tag = mats[1]
        _,note_text,err :=get_text(db_link, db_folder ,text_tag )
        if err ==nil{
            res_tags := extract_tags(note_text)
            for _,tag := range(res_tags){
                resource_ref_count_dec(db_link,tag)
                resource_link_del(db_link,tag,1,record.Tag)
            }
            // delete the text_resouce, decrease the ref_count
            // resource_ref_count_dec(db_link,text_tag)
            resource_link_del(db_link,text_tag,1,record.Tag)
            _,err:=resource_delete(db_link,text_tag,db_folder)
            if err !=nil{
                fmt.Printf("delete text resource error:%#v",err)
            }
        }
    }
    tab_note:=get_table("file_note")
    tab_note.set("file_dir",file_dir).set("file_name",file_name)
    _,err=do_delete(db_link,tab_note.pack_delete())
    if err !=nil{
        fmt.Printf("error:%#v",err)
        return false,err
    }
    tag_clear(db_link,record.Tag)
    return true,nil
}

func del_note_by_tag(db_link *sql.DB,note_tag string,db_folder string)(bool,error){
    record,err := get_note_by_tag(db_link,note_tag)
    if err !=nil{
        return false,err
    }
    reg:=regexp.MustCompile(`#<0x_([\d\w]+)_>`)
    mats:= reg.FindStringSubmatch(record.Note)
    var text_tag string
    // there are others to delete:
    // 1. the resource related to this note
    // 2. the blob
    if len(mats)>1{
        text_tag = mats[1]
        _,note_text,err :=get_text(db_link, db_folder ,text_tag )
        if err ==nil{
            res_tags := extract_tags(note_text)
            for _,tag := range(res_tags){
                resource_ref_count_dec(db_link,tag)
                resource_link_del(db_link,tag,1,record.Tag)
            }
             
            resource_link_del(db_link,text_tag,1,note_tag)          
            _,err:=resource_delete(db_link,text_tag,db_folder)
            if err !=nil{
                fmt.Printf("delete text resource error:%#v",err)
            }
        }
    }
    tab_note:=get_table("file_note")
    tab_note.set("tag",note_tag)
    _,err=do_delete(db_link,tab_note.pack_delete())
    if err !=nil{
        fmt.Printf("error:%s\n",err.Error())
        return false,err
    }
    tag_clear(db_link,note_tag)
    return true,nil
}

func edit_note(db_link *sql.DB,tag string,note string,color string,db_folder string)(bool,error){
    record,err := get_note_by_tag(db_link,tag)
    if err !=nil{
        fmt.Printf("error editing note:%s\n",err.Error())
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

            //update the names of the images in the new note
            //for tinyMCE editor already stored the images
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

    tab_note:=get_table("file_note")    
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
    tab_note:=get_table("file_note")
    tab_note.set("file_name",new_name).set("tag",note.Tag)
    check:=[]string{"tag"}
    _,err= do_update(db_link,tab_note.pack_update(check))
    if err !=nil{
        return false, err
    }
    return true,nil
}

func note_update_folder(db_link *sql.DB,file_dir string, file_name string,new_name string)(bool,error){
    if sys_delim() =="\\"{
        file_dir =strings.ReplaceAll(file_dir,"\\","/")
    }    
    note,err:=get_note_record(db_link,file_dir, file_name)
    
    if err!=nil {
        fmt.Printf("not found:file_dir:%s,file_name:%s\n",file_dir,file_name)
        return false, err
    }
    tab_note:=get_table("file_note")
    tab_note.set("file_dir",new_name).set("tag",note.Tag)
    check:=[]string{"tag"}
    _,err= do_update(db_link,tab_note.pack_update(check))
    if err !=nil{
        return false, err
    }
    return true,nil
}

func note_folder_like(db_link *sql.DB, folder_prefix string)([]Note_record,error){
    var result []Note_record
    sql_str  := "select tag,file_dir,file_name,note,color,ndate from file_note where file_dir like \""+folder_prefix+"%\""
    rows,err :=db_link.Query(sql_str)
    if err !=nil{
        return result,err
    }
    var row Note_record
    defer rows.Close()
    for rows.Next(){
        err =rows.Scan(&row.Tag,&row.File_dir,&row.File_name,&row.Note,&row.Color,&row.Ndate)
        if err !=nil{
            return result, err
        }
        result = append(result,row)
    }
    return result, nil
}

func note_change_path(db_link *sql.DB, folder_prefix string, new_prefix string)(bool, error){
    if folder_prefix==""{
        return false,errors.New("changing root_dir is not allowed")
    }
    records,err := note_folder_like(db_link, folder_prefix)
    if err !=nil{
        return false, err
    }

    tab:=get_table("file_note")
    check := []string{"tag"}
    for _,record:=range(records){
        if strings.HasPrefix(record.File_dir,folder_prefix){
            new_file_dir := strings.Replace(record.File_dir,folder_prefix,new_prefix,1)
            tab.set("tag",record.Tag).set("file_dir",new_file_dir)
            _,err :=do_update(db_link,tab.pack_update(check))
            if err!=nil{
                return false, err
            }
        }
    }
    return true,nil
}


func note_update_dirs(db_link *sql.DB, old_dir string,new_name string,root_dir string,delim string,full_path bool)(bool,error){
    // old_dir and root_dir is in native form
    old_name :=old_dir
    if strings.HasSuffix(old_dir,delim){
        old_name = old_dir[0:(len(old_dir)-1)] 
    }
    query_path:=relative_path_of(old_dir,root_dir)
    if delim=="\\"{
        //for windows
        query_path = strings.ReplaceAll(query_path,"\\","/")
    }
    sql_str  := "select tag,file_dir from file_note where file_dir like \""+query_path+"%\""
    row,err :=db_link.Query(sql_str)
    defer row.Close()
    if err !=nil{
        row.Close()
        return false,err
    }
    tag :=""
    file_dir :=""
    data :=make(map[string]string)
    for row.Next(){
        row.Scan(&tag,&file_dir)
        data[tag]=file_dir
    }
    
    name_len := len(path_file_name(old_name,delim))
    dir_prefix:=relative_path_of(path_dir_name(old_name,delim),root_dir)

    has_error:=false
    for k,v :=range(data){
        if strings.HasPrefix(v,dir_prefix){
            //guand against bad return of previous query
            var new_file_dir string
            if full_path{
                new_file_dir=new_name
            }else{
                idx_start:=len(dir_prefix)+name_len
                if idx_start >=len(v){
                    continue
                }
                tail := v[idx_start:len(v)]
                new_file_dir=dir_prefix+new_name+tail
            }
            if delim=="\\"{
                //for windows, in the file_note table, delim is normalized to "/"
                new_file_dir = strings.ReplaceAll(new_file_dir,"\\","/")
            }
            sql_str ="update file_note set file_dir=\""+new_file_dir +"\" where tag =\""+k+"\""
            sql_run,err :=db_link.Prepare(sql_str)
            if err !=nil{
                has_error =true
                fmt.Println("for Debug:"+sql_str)
                fmt.Println("error:"+err.Error())               
                continue
            }
            _,err=sql_run.Exec()
            if err !=nil{
                fmt.Println("error:"+err.Error()) 
                has_error =true
            }

        }
    }
    if has_error{
        return false,err
    }
    return true, nil
}

func get_note_map(db_link *sql.DB,device_id uint64,ino uint64,root_dir string,db_folder string) (map[string]Note_record, error){
    tab_note:=get_table("file_note")
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

func assign_note(db_link *sql.DB,note_tag string, dev_ino string, root_dir string)(bool,error){
    dev_id,ino,err:=dev_ino_uint64(dev_ino)
    sys_delim :=sys_delim()
    if err!=nil{
        return false,err
    }
    file_url,err := file_url(db_link,dev_id,ino,100,sys_delim)
    if err!=nil{
        return false,err
    }
    ok,err:=file_exists(file_url)
    if !ok || err!=nil{
        return false,errors.New("file not found")
    }
    file_url_rel := relative_path_of(file_url,root_dir)

    if sys_delim=="\\"{
        file_url_rel=strings.ReplaceAll(file_url_rel,"\\","/")
    }

    file_dir := path_dir_name(file_url_rel,sys_delim)
    file_name := path_file_name(file_url_rel,sys_delim)

    tab:=get_table("file_note")
    tab.set("tag",note_tag).set("file_dir",file_dir).set("file_name",file_name)
    check:=[]string{"tag"}
    _,err=do_update(db_link,tab.pack_update(check))
    if err !=nil{
        return false, err
    }
    return true,err
}


func color_decode(id int) string{
    coden_tab :=map[int]string{1:"green",2:"red",3:"blue",4:"purple",5:"orange",6:"yellow",7:"grey"}
    color,ok :=coden_tab[id]
    if !ok{
        return "default"
    }
    return color
}


// for ino_tree talbe and fs things
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
        refresh_folder(db_link,path,is_root)
    }
    if err !=nil{
        return false, err
    }
    return true,err

}

func file_rename(db_link *sql.DB,old_url string,new_name string,root_dir string)(bool,error){
    delim :=sys_delim()
 
    old_name := path_file_name(old_url,delim)
    dir := path_dir_name(old_url,delim)
    new_path := dir + new_name
    _,err := file_safe_mv(old_url,new_path,false) // no force 
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
    // for windows
    if delim=="\\"{
        file_dir=strings.ReplaceAll(file_dir,"\\","/")
    }

    _,err=note_update_name(db_link,file_dir,old_name,new_name)

    if err !=nil{
        if err.Error()=="no record"{
            return true,nil
        }else{
            return false, err
        }       
    }

    return true,nil  
}

func folder_rename(db_link *sql.DB,old_url,new_name string,root_dir string)(string,error){
    delim :=sys_delim()

    if old_url ==root_dir{
        return "",errors.New("root_dir rename is not allowed")
    }
    old_path := old_url
    if strings.HasSuffix(old_url,delim){
        old_path =old_url[0:(len(old_url)-1)]
    }    

    old_dir := path_dir_name(old_path,delim)
    new_path := old_dir + new_name +delim
    // old_url, old_path, old_dir,new_path is in native form

    _,err := file_safe_mv(old_url,new_path,false) // force off
    if err !=nil{
        return "",err
    }
    // handle ino tree
    refresh_folder(db_link,old_dir,old_dir==root_dir)
    
    // handle file_note
    _,err=note_update_dirs(db_link, old_url,new_name ,root_dir ,delim,false)

    if err !=nil{
        return "", err     
    }
    return new_path,nil
}

// for gin view--------------------------------------------------------
func Fnode_to_view(node *Fnode) *Fnode_view{
    var result Fnode_view
    result.Name,result.IsDir, result.Dev, result.Ino=node.Name,node.IsDir, node.Dev, node.Ino
    result.Parent_dev, result.Parent_ino =node.Parent_dev,node.Parent_ino
    return &result
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
// for shortcut and stash

type Shortcut_record struct{
    Scid int
    Track_id int
    File_dir string
    File_name string
    Sc_type string
    Order_id int
}
func add_shortcut(db_link *sql.DB, file_dir string, file_name string, sc_type string)(bool,error){
	if sys_delim()=="\\"{
        // in shortcut, delim is normalized to "/"
        file_dir=strings.ReplaceAll(file_dir,"\\","/")
    }    
    tab := get_table("shortcut")
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

func del_shortcut(db_link *sql.DB, file_dir string, file_name string,sc_type string)(bool,error){
    if sys_delim()=="\\"{
        file_dir=strings.ReplaceAll(file_dir,"\\","/")
    }   

    tab := get_table("shortcut")
	tab.set("file_dir",file_dir).set("file_name",file_name).set("type",sc_type)

	_,err :=do_delete(db_link,tab.pack_delete())
	if err !=nil{
		return false, err
	}
	return true,nil
}

func del_shortcut_id(db_link *sql.DB,scid string)(bool,error){
    tab := get_table("shortcut")
    tab.set("scid",scid)

    _,err :=do_delete(db_link,tab.pack_delete())
	if err !=nil{
		return false, err
	}
	return true,nil
}

func get_shortcut_map(db_link *sql.DB, file_url string, root_dir string)(map[string]string,error){
    tab := get_table("shortcut")
    rel_url:=relative_path_of(file_url,root_dir)
    file_dir := path_dir_name(rel_url,sys_delim())
    if sys_delim()=="\\"{
        file_dir=strings.ReplaceAll(file_dir,"\\","/")
    }  
    tab.set("file_dir",file_dir)
    rows,err:=db_link.Query(tab.pack_select("file_name,type","",""))
    defer rows.Close()
    result:=make(map[string]string)
    if err!=nil{
        return result,err
    }
    for rows.Next(){
        var name,sc_type string
        rows.Scan(&name,&sc_type)
        if name==""{
            if tmp,ok := result["000root000"];ok{
                result["000root000"]=tmp+sc_type 
            }else{
                result["000root000"]=sc_type
            }
            
        }else{
            if tmp,ok := result[name];ok{
                result[name]=tmp+sc_type
            }else{
                result[name]=sc_type
            }            
        }
    }
    return result,nil
}


func get_shortcut_map_folder(db_link *sql.DB, file_url string, root_dir string)(map[string]string,error){
    rel_dir := relative_path_of(file_url,root_dir) // here file_url should end with /
    if sys_delim()=="\\"{
        rel_dir = strings.ReplaceAll(rel_dir,"\\","/")
    }
    sql_str :=""
    if rel_dir == ""{
        sql_str ="select file_dir,type from shortcut where type in (\"d\",\"t\")" // all folders
    }else{
        sql_str = "select file_dir,type from shortcut where type in (\"d\",\"t\") and file_dir like \""+rel_dir+"%\""
    }
    rows,err:=db_link.Query(sql_str)
    defer rows.Close()
    result:=make(map[string]string)
    if err!=nil{
        return result,err
    }
    reg := regexp.MustCompile("[^/]+/$")
    for rows.Next(){
        var file_dir,sc_type string
        rows.Scan(&file_dir,&sc_type)
        if file_dir==""{
            continue            
        }else{
            t :=relative_path_of(file_dir,rel_dir)
            if !reg.MatchString(t){
                continue
            }
            if len(t)<2{
                continue
            }
            name := t[0:(len(t)-1)]
            if tmp,ok := result[name];ok{
                result[name]=tmp+sc_type
            }else{
                result[name]=sc_type
            }            
        }
    }
    return result,nil
}

func shortcut_entry(db_link *sql.DB,sc_type string,root_dir string)(string,error){
    tab := get_table("shortcut")
	tab.set("type",sc_type)
    var temp_list []string
    var file_dir string
    var file_name string
    var rst string 
    rows,err := db_link.Query(tab.pack_select("file_dir,file_name","",""))
    defer rows.Close()

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

func shortcut_list(db_link *sql.DB,sc_type string)([]Shortcut_record,error){
    tab := get_table("shortcut")
	tab.set("type",sc_type)
    var result []Shortcut_record
    var record Shortcut_record 
    rows,err := db_link.Query(tab.pack_select("scid,file_dir,file_name,type","",""))
    defer rows.Close()

    if err !=nil{
        return result,err
    }
    for rows.Next(){
        err:=rows.Scan(&record.Scid,&record.File_dir,&record.File_name,&record.Sc_type)
        if err !=nil{
            continue
        }
        result = append(result,record)
    }
    return result, err
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

func get_shortcut_records(db_link *sql.DB,file_dir string, file_name string)([]Shortcut_record,error){
    tab := get_table("shortcut")
	tab.set("file_dir",file_dir).set("file_name",file_name)
    var result []Shortcut_record    
    rows,err := db_link.Query(tab.pack_select("scid,file_dir,file_name,type","",""))
    defer rows.Close()

    if err !=nil{
        return result,err
    }
    var sc Shortcut_record
    for rows.Next(){
        err:=rows.Scan(&sc.Scid,&sc.File_dir,&sc.File_name,&sc.Sc_type)
        if err!=nil{
            fmt.Printf("error:%q\n",err)
            continue
        }
        result=append(result,sc)
    }        
    return result,err
}

func shortcut_rename_file(db_link *sql.DB,old_url string, new_name string,root_dir string)(bool,error){
    delim :=sys_delim()
    rel_url := relative_path_of(old_url,root_dir)
    file_dir := path_dir_name(rel_url,delim)
    file_name := path_file_name(rel_url,delim)
    if delim=="\\"{
        // for windows
        file_dir=strings.ReplaceAll(file_dir,"\\","/")
    }
    sc_records,err :=get_shortcut_records(db_link,file_dir,file_name)

    if err!=nil{
        return false, err
    }
    if len(sc_records)==0{
        return false, nil
    }
    tab := get_table("shortcut")
    for _,record :=range(sc_records){
        tab.set("scid",strconv.Itoa(record.Scid)).set("file_name",new_name)
        _,err:=do_update(db_link,tab.pack_update([]string{"scid"}))
        if err !=nil{
            fmt.Printf("error:%q\n",err)
            return false, err
        }
    }
    return true,nil
}

func shortcut_folder_like(db_link *sql.DB,folder_prefix string) ([]Shortcut_record,error){
    var result []Shortcut_record
    if folder_prefix==""{
        return result,errors.New("query empty")
    }
    sql_str := "select scid,file_dir,file_name,type from shortcut where file_dir like \""+folder_prefix+"%\""
    rows,err:=db_link.Query(sql_str)
    defer rows.Close()
    if err !=nil{
        return result,err
    }
    var row Shortcut_record
    for rows.Next(){
        err :=rows.Scan(&row.Scid,&row.File_dir,&row.File_name,&row.Sc_type)
        if err!=nil{
            continue
        }
        result =append(result,row)
    }
    return result,nil
}

func shortcut_change_path(db_link *sql.DB,folder_prefix string, new_prefix string)(bool,error){
    records,err :=shortcut_folder_like(db_link,folder_prefix)
    if err !=nil{
        return false,err
    }
    tab := get_table("shortcut")
    check:=[]string{"scid"}
    for _,record :=range(records){
        if strings.HasPrefix(record.File_dir,folder_prefix){
            new_file_dir := strings.Replace(record.File_dir,folder_prefix,new_prefix,1)
            tab.set("scid",strconv.Itoa(record.Scid)).set("file_dir",new_file_dir)            
            _,err:=do_update(db_link,tab.pack_update(check))
            if err !=nil{
                return false,err
            }
        }
    }
    return true,nil
}


func shortcut_rename_folder(db_link *sql.DB,old_url string, new_name string,root_dir string,full_path bool)(bool,error){
    rel_url := relative_path_of(old_url,root_dir)
    sys_delim :=sys_delim()
    file_dir := path_dir_name(rel_url,sys_delim)
    data := make(map[int]string)
    sql_str := "select scid,file_dir from shortcut where file_dir like \""+file_dir+"%\""
    rows,err:=db_link.Query(sql_str)
    defer rows.Close()

    if err!=nil{
        return false, err
    }
    
    var scid int
    var file_dir_t string
    for rows.Next(){
        err :=rows.Scan(&scid,&file_dir_t)
        if err !=nil{
            continue
        }
        data[scid]=file_dir_t
    }

    tab := get_table("shortcut")
    has_err :=false
    for scid,file_dir_t =range(data){
        if file_dir ==""{
            continue
        }
        var new_folder string
        if full_path{
            new_folder=new_name
        }else{
            old_path := file_dir
            if strings.HasSuffix(file_dir,sys_delim){
                old_path=old_path[0:(len(old_path)-1)]
            }
            old_path_prefix :=path_dir_name(old_path,sys_delim)
            var tail string
            if len(file_dir)<len(file_dir_t){
                tail =file_dir_t[len(file_dir):len(file_dir_t)]
            }else{
                tail =""
            }
            new_folder =old_path_prefix+new_name+sys_delim+tail
        }
        
        tab.set("scid",strconv.Itoa(scid)).set("file_dir",new_folder)
        _,err:=do_update(db_link,tab.pack_update([]string{"scid"}))
        if err !=nil{
            fmt.Printf("error updating:%q\n",err)
            has_err =true
        }
    }
    if has_err{
        return false, err
    }
    return true,nil
}



// for stash
func get_shortcut_by_id(db_link *sql.DB,scid string) (Shortcut_record,error){
    var result Shortcut_record
    tab := get_table("shortcut")
	tab.set("scid",scid)
    rows,err := db_link.Query(tab.pack_select("scid,file_dir,file_name,type","",""))
    defer rows.Close()
    if err!=nil{
        return result, err
    }
    if rows.Next(){
        rows.Scan(&result.Scid,&result.File_dir,&result.File_name,&result.Sc_type)
    }else{
        return result, errors.New("no record:scid"+scid)
    }
    return result,nil
}

func shortcut_update_folder(db_link *sql.DB,file_dir string, file_name string, new_name string)(bool,error){

    new_name = strings.ReplaceAll(new_name,"\"","\"\"")
    file_name = strings.ReplaceAll(file_name,"\"","\"\"")
    file_dir = strings.ReplaceAll(file_dir,"\"","\"\"")
    sql_str := "update shortcut set file_dir=\""+new_name+"\" where file_name=\""+file_name+"\" and file_dir=\""+file_dir+"\""
    sql_run,err:= db_link.Prepare(sql_str)
    if err !=nil{
        return false, err
    }
    _,err=sql_run.Exec()
    if err!=nil{
        return false,err
    }
    return true,nil
}

func stash_putdown(db_link *sql.DB,scid string,dev_ino string, root_dir string) (bool,error){
    delim :=sys_delim()
    stashed,err :=get_shortcut_by_id(db_link,scid)
    if err !=nil{
        return false, err
    }
    device_id,ino,err := dev_ino_uint64(dev_ino)
    if err !=nil{
        return false, err
    }
    url,err :=file_url(db_link,device_id,ino,100,delim)
    if err !=nil{
        return false, err
    }
    if !strings.HasSuffix(url,delim){
        return false,errors.New("not folder")
    }

    new_dir := relative_path_of(url, root_dir)
    switch stashed.Sc_type{
    case "s":
        // 1.move the file
        // err=os.Rename(root_dir+stashed.File_dir+stashed.File_name,url+stashed.File_name)
        _,err =file_safe_mv(str_native_delim(root_dir+stashed.File_dir+stashed.File_name),str_native_delim(url+stashed.File_name),false) // no force
        if err!=nil{
            return false,err
        }
        // 2. in file_note table, change folder
        _,err =note_update_folder(db_link,stashed.File_dir,stashed.File_name,str_db_delim(new_dir))
        if err !=nil && err.Error()!="no record"{
            return false,err
        }
        // 3. delete the stashed in the shortcut table
        _,err =del_shortcut(db_link,stashed.File_dir,stashed.File_name,"s")
        if err !=nil && err.Error()!="no record"{
            return false,err
        }
        // 4. in short_cut table,change folder
        _,err =shortcut_update_folder(db_link,stashed.File_dir,stashed.File_name, str_db_delim(new_dir))
        if err !=nil{
            return false,err
        }
    case "t":
              
        if !strings.HasSuffix(stashed.File_dir,"/"){
            return false,errors.New("folder format problem")
        }
        
        temp_path := stashed.File_dir[0:(len(stashed.File_dir)-1)]
        name := path_file_name(temp_path,"/")

        if strings.Contains(new_dir+name+"/",stashed.File_dir){
            return false, errors.New("moving folder into sub-folders not allowed")
        }
        // 1.move the file 
        _,err:=file_safe_mv(str_native_delim(root_dir+stashed.File_dir),str_native_delim(root_dir+new_dir+name+"/"),false)
        if err !=nil{
            return false,err
        }
        // 2. change the path in the file_note table
        _,err=note_change_path(db_link, stashed.File_dir,str_db_delim(new_dir+name+"/"))
        if err !=nil{
            return false,err
        }
        
        //3. delete the stash
        del_shortcut(db_link,stashed.File_dir,stashed.File_name,"t")

       //4. change the path of pin in the workspace
        _,err =shortcut_change_path(db_link, stashed.File_dir,str_db_delim(new_dir+name+"/"))
        if err !=nil{
            return false,err
        }
    default:
        fmt.Println("not supported sc_type")
        return false,err
    }
   
    return true,nil
}

// for settings
func has_setting(db_link *sql.DB,key string,note string) (bool,error){
    tab :=get_table("settings")
	tab.set("key",key)
    if note !=""{
        tab.set("note",note)
    }
	count,err := do_count(db_link,tab.pack_count("cnt"))
	if err !=nil{
		return false,err
	}
	if count ==0{
		return false,nil
	}
	return true,nil
}

func get_setting(db_link *sql.DB,key string,note string) (string,error){
	tab :=get_table("settings")
	tab.set("key",key)
    if note !=""{
        tab.set("note",note)
    }
	rows,err:=db_link.Query(tab.pack_select("value","",""))
	defer rows.Close()
	if err!=nil{
		return "",err
	}
	if rows.Next(){
		var value string
		rows.Scan(&value)
		return value,nil
	}
	return "",nil
}

func set_setting(db_link *sql.DB,key string, value string,note string)(bool,error){
	tab :=get_table("settings")
	tab.set("key",key).set("value",value)
	if note !=""{
		tab.set("note",note)
	}
	yes,err:= has_setting(db_link,key,note)
	if err!=nil{
		return false,err
	}
	if yes{
		update_sql:=""
		if note !=""{
			update_sql=tab.pack_update([]string{"key","note"})
		}else{
			update_sql=tab.pack_update([]string{"key"})
		}
		_,err:= do_update(db_link,update_sql)
		if err!=nil{
			return false,err
		}
		return true,nil
	}
	_,err= do_insert(db_link,tab.pack_insert())
	if err!=nil{
		return false,err
	}
	return true,nil
}
func clear_setting(db_link *sql.DB,key string, note string)(bool,error){
	tab :=get_table("settings")
	tab.set("key",key)
    if note !=""{
        tab.set("note",note)
    }
	_,err:=do_delete(db_link,tab.pack_delete())
	if err!=nil{
		return false,err
	}
	return true, nil
}

func set_host_setting(db_link *sql.DB,host_name string,key string,value string)(bool,error){
    return set_setting(db_link,key,value,host_name)
}

func get_host_setting(db_link *sql.DB,host_name string,key string,default_value string)(string){
    val,err:=get_setting(db_link,key,host_name)
    if err !=nil{
        return default_value
    }
    if val==""{
        return default_value
    }
    return val
}

func set_sys_setting(db_link *sql.DB,key string,value string)(bool,error){
    return set_setting(db_link,key,value,"sys")
}

func get_sys_setting(db_link *sql.DB,key string,default_value string)string{
    val,err:=get_setting(db_link,key,"sys")
    if err !=nil{
        return default_value
    }
    if val==""{
        return default_value
    }
    return val
}

func set_host_root(db_link *sql.DB,host_name string,root_dir string)(bool,error){
	return set_setting(db_link,"root_dir",root_dir,host_name)
}

func get_host_root(db_link *sql.DB,host_name string)(string,error){
	return get_setting(db_link,"root_dir",host_name)
}

func set_page_wrap_class(db_link *sql.DB,host_name string,cls string)(bool,error){
	return set_setting(db_link,"wrap_class",cls,host_name)
}
func get_page_wrap_class(db_link *sql.DB,host_name string)(string){
	return get_host_setting(db_link,host_name,"wrap_class","content_wrap")
}

func get_setting_with_digit(db_link *sql.DB,key string,default_val int)int{
    len_str := get_sys_setting(db_link,"img_page_len","")
    if len_str ==""{
        return default_val // default
    }
    len_int,err := strconv.Atoi(len_str)
    if err!=nil{
        return default_val
    }
    return len_int
}

func get_img_page_len(db_link *sql.DB)int{
    return get_setting_with_digit(db_link,"img_page_len",20)
}

func set_img_page_len(db_link *sql.DB,length string)(bool,error){
    return set_sys_setting(db_link,"img_page_len",length)
}


func get_article_list_len(db_link *sql.DB)int{
    return get_setting_with_digit(db_link,"article_list_len",50)
}

func set_article_list_len(db_link *sql.DB,length string)(bool,error){
    return set_sys_setting(db_link,"article_list_len",length)
}


func get_notes_page_len(db_link *sql.DB)int{
    return get_setting_with_digit(db_link,"notes_page_len",50)
}

func set_notes_page_len(db_link *sql.DB,length string)(bool,error){
    return set_sys_setting(db_link,"notes_page_len",length)
}

func set_db_version(db_link *sql.DB,version string)(bool,error){
    return set_setting(db_link,"db_version",version,"sys")
}

func get_host_opener(db_link *sql.DB,file_type string)(string){
    opener:= get_host_setting(db_link,get_host_name(),file_type+"_opener","")
    if opener !=""{
        return opener
    }
    os_type :=runtime.GOOS
    var default_opener=make( map[string]string)
    switch(os_type){
    case "darwin":
        return "open"
    case "linux":
        default_opener["pdf"]="open"
        default_opener["docx"]="open"
        default_opener["doc"]="open"
        default_opener["pptx"]="open"
        default_opener["ppt"]="open"
    case "windows":
        default_opener["ppt"]="open"
    default:

    }
    default_value,ok :=default_opener[file_type]
    if !ok{
        default_value=""
    } 
    return default_value
}

func set_host_opener(db_link *sql.DB,host_name string,file_type string,opener_path string)(bool,error){
    return set_host_setting(db_link,host_name,file_type+"_opener",opener_path)
}

func enum_host_openers(db_link *sql.DB,host_name string)(string,error){
    tab :=get_table("settings")
    tab.set("note",host_name).set("key","%_opener")
    rows,err:=db_link.Query(tab.pack_select("key,value","",""))
    if err!=nil{
        return "",err
    }
    result:=""
    for rows.Next(){
        k:=""
        v:=""
        rows.Scan(&k,&v)
        t:=strings.Split(k,"_")
        result=result+t[0]+"="+v+"\n"
    }
    return result,nil
}

func init_settings(db_link *sql.DB)error{
    host_name := get_host_name()
    _,err:=set_db_version(db_link,"0.2") // 2022-5-24
    if err!=nil{
        return err
    }
    _,err=set_page_wrap_class(db_link,host_name,"content_wrap")
    if err!=nil{
        return err
    }
    _,err=set_img_page_len(db_link,"20")
    if err!=nil{
        return err
    }
    return err
}


// =========================
//for article table        
//==========================

type Article_record struct{
    Artid int64
    Tag string
    Shelf_id int
    Title string
    Adate string
    Color string
}

type Article_page_record struct{
    Pgid int64
    Pg_tag string
    Tag string
    Order_id int
    Pdate string
    Data string
}

func new_article(db_link *sql.DB,title string,color string,shelf_id string)(string,error){
    tab := get_table("article")
    tag,err := tag_gen(db_link)
    if err !=nil{
        return tag,err
    }
    if shelf_id==""{
        shelf_id="0"
    }
    if color==""{
        color="7" // grey
    }

    tab.set("title",title).set("tag",tag).set("color",color).set("shelf_id",shelf_id).set("adate",get_now_string())
    _,err =do_insert(db_link,tab.pack_insert())
    if err !=nil{
        return tag,err
    }
    return tag,nil
}

func count_articles(db_link *sql.DB)(int64, error){
    tab := get_table("article")
    count,err := do_count(db_link,tab.pack_count("cnt"))
    if err !=nil{
        return 0,err
    }
    return count,nil
}


func list_articles(db_link *sql.DB,page int,page_len int)([]Article_record,error){
    var result []Article_record
    cnt,err := count_articles(db_link)
    if err!=nil{
        return result,errors.New("count error")
    } 
    if page_len <1{
        return result,errors.New("page len error")
    }
    page_count:= calc_pages(cnt, page_len)

    if page>int(page_count){
        page = int(page_count)
    }
    if page <1{
        page =1
    }
    start :=(page-1)*page_len
    tab := get_table("article")
    rows,err:=db_link.Query(tab.pack_select("tag,shelf_id,title,adate,color","artid desc",strconv.Itoa(start)+","+strconv.Itoa(page_len)))
    defer rows.Close()

    if err !=nil{
        return result, err
    }
    var record Article_record
    for rows.Next(){
        rows.Scan(&record.Tag,&record.Shelf_id,&record.Title,&record.Adate,&record.Color)
        result = append(result,record)
    }
    return result,nil
}

func search_article(db_link *sql.DB,target string)([]Article_record,error){
    var result []Article_record
    sql_str :="select tag,shelf_id,title,adate,color from article where title like \"%"+target+"%\" order by  artid desc"
    fmt.Println(sql_str)
    rows,err:=db_link.Query(sql_str)
    defer rows.Close()
    if err !=nil{
        return result, err
    }
    var record Article_record
    for rows.Next(){
        rows.Scan(&record.Tag,&record.Shelf_id,&record.Title,&record.Adate,&record.Color)
        result = append(result,record)
    }
    return result,nil    
}

func update_article(db_link *sql.DB,tag string,new_title string,color string,shelf_id string)(bool,error){
    tab := get_table("article")
    tab.set("title",new_title).set("tag",tag)
    if color !=""{
        tab.set("color",color)
    }
    if shelf_id !=""{
        tab.set("shelf_id",shelf_id)
    }

    check :=[]string{"tag"}
    _,err:= do_update(db_link,tab.pack_update(check))
    if err!=nil{
        return false,err
    }
    return true,nil
}

func del_article(db_link *sql.DB,tag string,db_folder string)(bool,error){
    tab := get_table("article")
    tab.set("tag",tag)

    pages,err:=get_page_records_by_tag(db_link,tag)
    if err !=nil{
        return false,err
    }
    for _,article_page :=range(pages){
        _,err=del_article_page(db_link,article_page.Pg_tag,db_folder)
        if err!=nil{
            fmt.Println("error deleting article page:"+err.Error())
        }
    }
    _,err=do_delete(db_link,tab.pack_delete())
    if err!=nil{
        return false,err
    }
    tag_clear(db_link,tag)
    return true,nil
}

func get_article_record(db_link *sql.DB,tag string)(Article_record, error){
    var record Article_record
    tab := get_table("article")
    tab.set("tag",tag)
    rows,err:=db_link.Query(tab.pack_select("tag,shelf_id,title,adate,color","artid desc",""))
    defer rows.Close()

    if err !=nil{
        return record, err
    }
   
    if rows.Next(){
        err:=rows.Scan(&record.Tag,&record.Shelf_id,&record.Title,&record.Adate,&record.Color)
        if err !=nil{
            return record,err
        }
        return record,nil
    }
    return record,errors.New("no record")
}


func add_article_page(db_link *sql.DB,tag string,note string,db_folder string)(string,error){
    tab := get_table("article_page")
  
    // insert in the blob
    blob_tag,err:=resource_deposite(db_link,tag,33,[]byte(note),db_folder)
    // tag is the article tag
    if err !=nil{
        return "",err
    }

    // pg_tag is the same as blob_tag
    tab.set("pg_tag",blob_tag).set("tag",tag).set("pdate",get_now_string()).set("order_id","0")
    _,err = do_insert(db_link,tab.pack_insert())
    if err !=nil{
        return "",err
    }
    resource_ref_count_inc(db_link,blob_tag)
    resource_link_add(db_link,blob_tag, 2, tag)
    new_tags := extract_tags(note)
    new_tags_map :=extract_img_names(note)
    for _,item :=range(new_tags){
        item_name,ok:=new_tags_map[item]
        if ok{
            if item_name == ""{
                continue
            }
            resource_update_name(db_link,item,item_name)
            resource_ref_count_inc(db_link,item)
            resource_link_add(db_link,item,2,tag)
        }
    }
    return blob_tag,nil  // blob_tag is the same as pg_tag in the article_page table
}

func edit_article_page(db_link *sql.DB,pg_tag string,note string,db_folder string)(bool,error){
    record,err:=get_page_by_pg_tag(db_link,pg_tag)
    if err !=nil{
        return false,err
    }
    
    _,old_note,err:=get_text(db_link, db_folder ,pg_tag )
    if err ==nil{
        //update the resource track in the text
        resource_ref_update_by_text(db_link,2,record.Tag,old_note,note)
    }
    
    // update the text resource
    _,err=resource_update(db_link,pg_tag,33,[]byte(note),db_folder)
    if err !=nil{
        return false,err
    }
    return true,nil
}

func article_page_set_order(db_link *sql.DB,pg_tag string,order_str string)(bool,error){
    tab := get_table("article_page")
    tab.set("pg_tag",pg_tag).set("order_id",order_str)
    check:=[]string{"pg_tag"}
    _,err :=do_update(db_link,tab.pack_update(check))
    if err !=nil{
        return false,err
    } 
    return true,nil
}

func del_article_page(db_link *sql.DB,pg_tag string,db_folder string)(bool,error){
    // pg_tag is the same as blob_tag
    record,err:=get_page_by_pg_tag(db_link,pg_tag)
    if err !=nil{
        return false,err
    }
    _,old_note,err:=get_text(db_link, db_folder,pg_tag)
    
     // 解除app_tag(article_tag,即record.Tag) 与old_note 中所有资源的引用连接
    resource_ref_dec_by_text(db_link,2,record.Tag,old_note)
    resource_link_del(db_link,pg_tag,2,record.Tag) // text 资源的引用连接解除
    resource_delete(db_link,pg_tag ,db_folder) //删除text资源
    tab := get_table("article_page")
    tab.set("pg_tag",pg_tag)
    _,err= do_delete(db_link,tab.pack_delete()) //删除page表中记录
    if err!=nil{
        return false,err
    }
    return true,nil
}


func list_article_pages(db_link *sql.DB,tag string,db_folder string)([]Article_page_record,error){
    var result []Article_page_record
    tab := get_table("article_page")
    tab.set("tag",tag)
    rows,err :=db_link.Query(tab.pack_select("pgid,pg_tag,tag,order_id,pdate","order_id desc,pgid asc",""))
    defer rows.Close()
    if err !=nil{
        return result,err
    }
    var record Article_page_record
    for rows.Next(){
        err=rows.Scan(&record.Pgid,&record.Pg_tag,&record.Tag,&record.Order_id,&record.Pdate)
        if err !=nil{
            continue
        }
        _,note,err:=get_text(db_link,db_folder ,record.Pg_tag)
        if err ==nil{
            record.Data=note
        }

        result=append(result,record)
    }
    return result,err
}

func get_page_by_pg_tag(db_link *sql.DB,pg_tag string)(Article_page_record,error){
    tab := get_table("article_page")
    tab.set("pg_tag",pg_tag)
    rows,err :=db_link.Query(tab.pack_select("pgid,pg_tag,tag,order_id,pdate","",""))
    defer rows.Close()
    var record Article_page_record
    if !rows.Next(){
        return record,errors.New("no record")
    }
    err=rows.Scan(&record.Pgid,&record.Pg_tag,&record.Tag,&record.Order_id,&record.Pdate)
    if err!=nil{
        return record,err
    }
    return record,nil
}

func get_page_records_by_tag(db_link *sql.DB,tag string)([]Article_page_record,error){
    var result []Article_page_record

    tab := get_table("article_page")
    tab.set("tag",tag)
    rows,err :=db_link.Query(tab.pack_select("pgid,pg_tag,tag,order_id,pdate","",""))
    defer rows.Close()
    if err !=nil{
        return result,err
    }
    var record Article_page_record
    for rows.Next(){
        err=rows.Scan(&record.Pgid,&record.Pg_tag,&record.Tag,&record.Order_id,&record.Pdate)
        if err !=nil{
            continue
        }
        result = append(result,record)
    }
    return result,nil
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
create table IF NOT EXISTS shortcut(scid INTEGER PRIMARY KEY AUTOINCREMENT,track_id INT,file_dir VARCHAR(250), file_name VARCHAR(250),type CHAR(1),order_id INTERGER);
create table IF NOT EXISTS article(artid INTEGER PRIMARY KEY AUTOINCREMENT, tag CHAR(10), shelf_id INT UNSIGNED,title VARCHAR(250), adate DATETIME, color CHAR(1));
create table IF NOT EXISTS article_page(pgid INTEGER PRIMARY KEY AUTOINCREMENT,pg_tag CHAR(10), tag CHAR(10),order_id SMALLINT UNSIGNED,pdate DATETIME);
create table IF NOT EXISTS settings(id INTEGER PRIMARY KEY AUTOINCREMENT,key VARCHAR(100),value VARCHAR(250), note VARCHAR(250) );
create index IF NOT EXISTS idx_dev_ino on ino_tree(host_name,device_id, ino);
create index IF NOT EXISTS idx_dev_parent on ino_tree(host_name,device_id,parent_ino);
create index IF NOT EXISTS idx_dev_ino_note on file_note(tag);
create index IF NOT EXISTS idx_file_note_path on file_note(file_dir,file_name);
create index IF NOT EXISTS idx_resource_tag on resource(tag);
create index IF NOT EXISTS idx_resource_link_tag on resource_link(tag);
create index IF NOT EXISTS idx_tags on tags(tag_str);
create index IF NOT EXISTS idx_article_tag on article(tag);
create index IF NOT EXISTS idx_article_title on article(title);
create index  IF NOT EXISTS idx_article_page_pg_tag on article_page(pg_tag);
create index  IF NOT EXISTS idx_article_page_pg_tag on article_page(tag);
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
const app_usage =`usage: Filegai [options] Folder
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
    host_name := get_host_name()

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
        db, err := get_db(db_file)
        if err !=nil{
            fmt.Println("?? error opening database file:",db_file)
            os.Exit(1) 
        }

        init_settings(db)
        db.Close()
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
    r.StaticFile("/favicon.ico","./public/favicon.ico")

    r.SetFuncMap(template.FuncMap{
        "unescapeHtmlTag":unescapeHtmlTag,
    })
    // r.LoadHTMLFiles("templates/index.html", "templates/notes.html","templates/images.html",
    //                 "templates/show_code.html","templates/status.html")

    r.LoadHTMLGlob("templates/*")
    r.GET("/",func(c *gin.Context){  
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        
        root_dir_in_db,err :=get_host_root(db,get_host_name())
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        if root_dir_in_db ==""{
            _,err :=set_host_root(db,get_host_name(),root_dir)
            if err!=nil{
                fmt.Println("?? setting has root error"+err.Error())
            }
        }else{
            if root_dir_in_db != root_dir{
                c.Redirect(http.StatusTemporaryRedirect,"/error/9")
            }
        }

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
    });

    r.GET("/list",func(c *gin.Context){ 
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }

        refresh_folder(db,root_dir,true)
        this_fnode,_ := get_Fnode(root_dir,true)
        c.Redirect(http.StatusTemporaryRedirect,"/list/"+strconv.FormatUint(uint64(this_fnode.Dev),10)+"_"+strconv.FormatUint(this_fnode.Ino,10))
    });

    r.GET("/nav/:dev_ino",func(c *gin.Context){ 
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }

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
      
    });

    r.GET("/list/:ino", func(c *gin.Context) {
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }

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
        if err !=nil{
            fmt.Println("?? error: pasing query inode")
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
            return
        }
        var active_device_id uint64
        var active_ino uint64
        if len(pairs)>1{
            active_device_id,active_ino,err=dev_ino_uint64(pairs[1])
            if err !=nil{
                active_device_id=0
                active_ino=0
            }
        }
        
        url,err := file_url(db,device_id,ino,100,sys_delim())
        if err !=nil{
            fmt.Println("?? error:geting file_url by query inode:"+err.Error())
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
            return
        }
        all_nodes :=folder_entries(url)
        sort.Sort(byAlpha(all_nodes))
        is_root :=false
        if url==root_dir{
            is_root = true
        }
        refresh_folder(db,url,is_root)
        this_node,err:=query_fnode(db,device_id, ino)
        if err !=nil{
            fmt.Println("?? error: query_fnode")
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
            return
        }

        var folder_nodes []*Fnode_view
        var file_nodes []*Fnode_view
        var shortcut_value string
        var shortcut_icon string
        var stash_value string
        var stash_class string

        notes_map,err:=get_note_map(db,device_id,ino,root_dir,db_folder)
        shortcut_map,err:=get_shortcut_map(db,url,root_dir)
        if err!=nil{
            fmt.Printf("error:getting shortcut map %q\n",err)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        shortcut_map_folder,err:=get_shortcut_map_folder(db,url,root_dir)
        if err!=nil{
            fmt.Printf("error:getting shortcut map folder %q\n",err)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }

        // this_folder :=path_file_name(relative_path_of(url,root_dir))
        sc_type,ok :=shortcut_map["000root000"]
        if ok{
            if strings.Contains(sc_type,"d"){
                shortcut_value="false"
                shortcut_icon="layui-icon-rate-solid"
            }else{
                shortcut_value="true"
                shortcut_icon="layui-icon-rate"
            }

            if strings.Contains(sc_type,"t"){
                stash_value="false"
                stash_class="stashed"
            }else{
                stash_value="true"
                stash_class="unstashed"
            }

        }else{
            shortcut_value="true"
            shortcut_icon="layui-icon-rate"
            stash_value ="true"
            stash_class ="unstashed"
        }
        for _,tmp_node :=range(all_nodes){
            // iterate through all the nodes, adding some viewing content
            fnv:=Fnode_to_view(tmp_node)
            if tmp_node.IsDir {
                sc_type,ok =shortcut_map_folder[tmp_node.Name]
                if ok{
                    if strings.Contains(sc_type,"d"){
                        fnv.Pin_class="pinned_folder"
                    }else{
                        fnv.Pin_class="unpinned_folder"
                    }
                    if strings.Contains(sc_type,"t"){
                        fnv.Stash_class="stashed_folder"
                    }else{
                        fnv.Stash_class="unstashed_folder"
                    }
                }else{
                    fnv.Pin_class="unpinned_folder"
                    fnv.Stash_class="unstashed_folder"
                }
                folder_nodes=append(folder_nodes,fnv)
            }else{
                
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
                sc_type,ok =shortcut_map[tmp_node.Name]
                if ok{
                    if strings.Contains(sc_type,"f"){
                        fnv.Pin_class="pinned"
                        fnv.Pin_value="false"
                    }else{
                        fnv.Pin_class="unpinned"
                        fnv.Pin_value="true"
                    }

                    if strings.Contains(sc_type,"s"){
                        fnv.Stash_class="stashed"
                        fnv.Stash_value="false"
                    }else{
                        fnv.Stash_class="unstashed"
                        fnv.Stash_value="true"
                    }
                }else{
                    fnv.Pin_class="unpinned"
                    fnv.Pin_value="true"
                    fnv.Stash_class="unstashed"
                    fnv.Stash_value="true"
                }
                file_nodes = append(file_nodes,fnv)
            }
        }        
    
        workspace_folders,err:=shortcut_entry(db,"d",root_dir)
        if err !=nil{
            fmt.Printf("error:getting shortcut folder entries %q\n",err)
            workspace_folders=""
        }
        workspace_files,err:=shortcut_entry(db,"f",root_dir)
        if err !=nil{
            fmt.Printf("error:getting shortcut file entries %q\n",err)
            workspace_files=""
        }
        var folder_name_maxlen=30
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
            "folder_name":this_node.Name,
            "folder_nodes":folder_nodes,
            "file_nodes":file_nodes,
            "url":url,
            "dev_ino":dev_ino,
            "parent_dev_ino":dev_ino_pair[0]+"_"+strconv.FormatUint(this_node.Parent_ino,10),
            "shortcut_value":shortcut_value,
            "shortcut_icon":shortcut_icon,
            "stash_value":stash_value,
            "stash_class":stash_class,
            "workspace_folders":workspace_folders,
            "workspace_files":workspace_files,
            "wrap_class":get_page_wrap_class(db,get_host_name()),
        })
    });

    // ============= RESOURCE HANDLE =====================
    r.POST("/image_upload",func(c *gin.Context){
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        
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
        img_suffix:=mime_decode_suffix(rs_type)
        tag,err:=resource_deposite_file(db,file.Filename,rs_type,db_folder+"upload_temp" ,db_folder)
        if err !=nil{
            c.JSON(http.StatusOK, gin.H{
                "location":"/get_image/xxx",
            })
        }else{
            c.JSON(http.StatusOK, gin.H{
                "location":"/get_image/"+tag+"."+img_suffix,
            })
        }
    });

    r.POST("/image_update",func(c *gin.Context){
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.String(http.StatusOK,"?? error opening the db") 
        }
        file, _ := c.FormFile("file")
        tag :=img_name_tag(c.PostForm("tag"))   
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
        defer db.Close()
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/public/css/sorry.png")
        }
        tag:=img_name_tag(c.Param("tag"))
        _,data,err:=get_image(db, db_folder,tag)
        if err!=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/public/css/sorry.png")
        }else{
            mime_str,_ := get_image_mime(db,tag)
            c.Data(http.StatusOK,mime_str,data)
        }
    });

    r.GET("/get_image_r/:tag",func(c *gin.Context){
        // for change image, after changing the image,
        // if src does not change, the image do not get updated
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/public/css/sorry.png")
        }
        tag:=img_name_tag(c.Param("tag"))
        _,data,err:=get_image(db, db_folder,tag)
        if err!=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/public/css/sorry.png")
        }else{
            mime_str,_ := get_image_mime(db,tag)
            c.Data(http.StatusOK,mime_str,data)
        }
    });

    r.GET("/list_image/:page",func(c *gin.Context){
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }

        page,_ :=strconv.Atoi(c.Param("page"))
        page_len := get_img_page_len(db)
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

    r.POST("/search_images",func(c *gin.Context){
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }

        target:=c.PostForm("target")
        images,err :=search_images(db,target)
        if err !=nil{
            fmt.Println("error search image:"+err.Error())
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }

        c.HTML(http.StatusOK,"images.html",gin.H{
            "images":images,
            "page_bar":"",
        })
    })

    r.GET("/orphan_images",func(c *gin.Context){
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }

        images :=orphan_images(db)

        c.HTML(http.StatusOK,"image_orphans.html",gin.H{
            "images":images,
            "page_bar":"",
        })
    });

    r.GET("/retrace_image/:tag",func(c *gin.Context){
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        var found=false;
        search_tag:=img_name_tag(c.Param("tag"))
        n_records,err:=search_notes(db,db_folder,search_tag )
        
        if err ==nil{
            for _,record:=range(n_records){
                _,err1:=resource_link_add(db,search_tag,1,record.Tag)
                if err1 ==nil{
                    resource_ref_count_inc(db,search_tag)
                    found=true
                }
            }
        }
        // a_records,err =search_article_page(db,target)
        // implement later

        if found{
            c.String(http.StatusOK,"!!Done")
            return 
        }
        c.String(http.StatusOK,"??failed")

    })

    r.POST("/image_cname",func(c *gin.Context){
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.String(http.StatusOK,"?? error open db")
        }
        
        tag :=img_name_tag(c.PostForm("tag"))
        name :=c.PostForm("new_name")
        ok,err :=resource_update_name(db,tag,name)
        if err !=nil{
            c.String(http.StatusOK,"??Data base error:"+err.Error())
            return 
        }
        if !ok{
            c.String(http.StatusOK,"??not Changed")
            return
        }
        c.String(http.StatusOK,"!!"+tag)
    });

    r.GET("/track/:tag",func(c *gin.Context){
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        
        tag :=img_name_tag(c.Param("tag"))
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
        }else if app==2{
            c.Redirect(http.StatusTemporaryRedirect,"/show_article/"+app_tag)
        }else{
            c.Redirect(http.StatusTemporaryRedirect,"/error/2")
        }
    });

    r.GET("/clear/:tag",func(c *gin.Context){
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.String(http.StatusOK,"?? error open db"+err.Error())
        }

        tag :=img_name_tag(c.Param("tag"))
        ok,err:=resource_delete(db,tag,db_folder)
        if err !=nil{
            c.String(http.StatusOK,"?? Data base error:"+err.Error())
            return
        }
        if !ok{
            c.String(http.StatusOK,"??not Changed")
            return
        }
        c.String(http.StatusOK,"!!"+tag)
    });

    //====================== NOTES HANDLE ======================
    r.POST("/add_note/:ino",func(c *gin.Context){
        // posting: 'ino_id' : ino_id,'tag': item_value, 'note':tinyMCE.get('note_content').getContent(),'color': color_code}
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.String(http.StatusOK,"?? error open db")
            return
        }

        ok,err:=regexp.MatchString(`\d+_\d+`,c.PostForm("ino_id"))

        if err!=nil{
            c.String(http.StatusOK,"?? query error,error compiling regexp")
            return
        }
        if !ok{
            c.String(http.StatusOK,"?? query error,error compiling regexp")
            return
        }
        pairs:=strings.Split(c.PostForm("ino_id"),"_")
        note:=c.PostForm("note")
        color:=c.PostForm("color")
        device_id := pairs[0]
        ino:=pairs[1]
        tag,err :=add_note(db,"virtual",device_id,ino,note,color,root_dir,db_folder)
        
        if err !=nil{
            c.String(http.StatusOK,"??error adding note-code")
            return
        }else{
            c.String(http.StatusOK,"!!"+tag)
            return
        }   

    });

    r.GET("/del_note/:ino",func(c *gin.Context){
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.String(http.StatusOK,"?? error open db")
            return
        }
        
        if strings.Contains(c.Param("ino"),"_"){
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
        }else{
            tag :=c.Param("ino")
            _,err :=del_note_by_tag(db,tag,db_folder)
            if err!=nil{
                c.String(http.StatusOK,"??del note failed")
            }else{
                c.String(http.StatusOK,"!!"+tag)
            }
        }
    });

    r.POST("/edit_note/:tag",func(c *gin.Context){
       // posting {'ino_id' : ino_id,'tag': item_value, 'note':tinyMCE.get('note_content').getContent(),'color': color_code}
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.String(http.StatusOK,"?? error open db")
            return
        }
        
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

    // handling rename
    r.POST("/rename/:ino",func(c *gin.Context){
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.String(http.StatusOK,"?? error open db")
            return
        }
         
        dev_ino:=c.PostForm("ino_id")
        // filter out illegal signs in the new file name
        reg := regexp.MustCompile(`[\*\.\?<>:"]`)
        new_name :=reg.ReplaceAllString(c.PostForm("new_name"),"_")+"."+c.PostForm("new_name_ext")
        device_id,ino,err :=dev_ino_uint64(dev_ino)
        if err !=nil{
            c.String(http.StatusOK,"??Query error,dev_ino:"+dev_ino+err.Error())
            return
        }

        old_url,err :=file_url(db,device_id,ino, 100,sys_delim() )
        // old_url :its delim is in native form

        if err !=nil{
            c.String(http.StatusOK,"??Getting file_url error:"+err.Error())
            return
        }

        _,err=file_rename(db,old_url,new_name,root_dir)
        if err !=nil{
            c.String(http.StatusOK,"??rename_error:"+err.Error())
            return 
        }
        _,err = shortcut_rename_file(db,old_url,new_name,root_dir )
        if err !=nil{
            c.String(http.StatusOK,"??shortcut_rename_error:"+err.Error())
            return
        }
        c.String(http.StatusOK,"!!new_name:"+new_name)
    });

    // handling rename_folder
    r.POST("/rename_folder",func(c *gin.Context){
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.String(http.StatusOK,"??error opening database file")
            return
        }
         
        dev_ino:=c.PostForm("ino_id")
        reg := regexp.MustCompile(`[\*\.\?<>:"]`)
        new_name :=reg.ReplaceAllString(c.PostForm("new_name"),"_")
        device_id,ino,err :=dev_ino_uint64(dev_ino)
        if err!=nil{          
            fmt.Printf("parsing error:%s\n",err.Error())
            c.String(http.StatusOK,"??parsing error")
            return
        }
        old_url,err :=file_url(db,device_id,ino, 100,sys_delim() )
        if err !=nil{
            fmt.Printf("error:%q\n",err)
            c.String(http.StatusOK,"??getting file_url error")
            return 
        }
        new_url,err:=folder_rename(db,old_url,new_name ,root_dir)
        if err !=nil{
            fmt.Printf("error:%q\n",err)
            c.String(http.StatusOK,"??parsing error")
            return
        }
        // db_link *sql.DB,old_url string, new_name string,root_dir string
        shortcut_rename_folder(db,old_url,new_name ,root_dir,false)
        c.String(http.StatusOK,"!!"+new_url)        

    })

    r.GET("/file_notes/:page",func(c *gin.Context){
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
                
        page,_ :=strconv.Atoi(c.Param("page"))
        page_len := get_notes_page_len(db)
        cnt,err := notes_count(db)
        err_msg:=""
        has_err:=false
        var all_notes []Note_record
        if err !=nil{
            has_err =true
            err_msg="count note error"
           
        }else{
            all_notes,err =list_notes(db, page_len,page,db_folder)
            if err !=nil{
                has_err =true
                err_msg +=" can't find note"
            }
        }

        if has_err{
            c.HTML(http.StatusOK,"error.html",gin.H{
                "error_msg":err_msg,
            })
        }else{
            var page_count int
            if int(cnt)%page_len==0{
                page_count = int(cnt)/page_len
            }else{
                page_count = int(cnt)/page_len+1
            }
            c.HTML(http.StatusOK,"notes.html",gin.H{
                "notes":all_notes,
                "page_bar":draw_page_bar(page_count,page,"background-color:#1E9FFF","/file_notes/"),
                "wrap_class":get_page_wrap_class(db,host_name),
            })
        }
    });

    r.GET("/orphan_notes",func(c *gin.Context){
        // func (db_link *sql.DB,db_folder string,root_dir string)([]Note_record,error){
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }

        notes,err := orphan_notes(db,db_folder,root_dir)
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }

        c.HTML(http.StatusOK,"note_orphans.html",gin.H{
            "notes":notes,
            "page_bar":"",
            "wrap_class":get_page_wrap_class(db,host_name),
        })    
    });

    r.POST("/search_note",func(c *gin.Context){
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
            return
        }
        
        target := c.PostForm("target")
        all_notes,err :=search_notes(db,db_folder,target)
        if err !=nil{
            if err.Error()=="no record"{
                c.HTML(http.StatusOK,"notes.html",gin.H{
                    "notes":all_notes,
                    "page_bar":"",
                    "wrap_class":get_page_wrap_class(db,host_name),
                })
            }else{
                c.HTML(http.StatusOK,"error.html",gin.H{
                    "error_msg":err.Error(),
                })
            }
        }else{
            c.HTML(http.StatusOK,"notes.html",gin.H{
                "notes":all_notes,
                "page_bar":"",
                "wrap_class":get_page_wrap_class(db,host_name),
            })
        }
    });

    r.POST("/retrace_note",func(c *gin.Context){
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.String(http.StatusOK,"?? open db failed")
            return
        }

        file_name:=c.PostForm("file_name")
        fnodes,err := search_fnodes(db,host_name,file_name)
        if err !=nil{
            c.String(http.StatusOK,"?? error db operatoin")
        }

        result:=""
        for _,fnode:=range(fnodes){
            result = result+"<p><input type='radio'  name='dev_ino' value='"
            result = result+strconv.FormatUint(uint64(fnode.Dev),10)+"_"+strconv.FormatUint(fnode.Ino,10)+"' />"+fnode.Name+"</p>\n"
        }
        c.String(http.StatusOK,"!!"+result)
    })

    r.POST("/assign_note",func(c *gin.Context){
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.String(http.StatusOK,"?? open db failed")
            return
        }

        note_tag:=c.PostForm("note_tag")
        dev_ino :=c.PostForm("dev_ino")
        _,err:=assign_note(db,note_tag,dev_ino,root_dir)
        if err!=nil{
            c.String(http.StatusOK,"?? error operating database")
            return
        }
        
        c.String(http.StatusOK,"!!Done")
    })

    // ============== handle article =======================
    r.POST("/new_article",func(c *gin.Context){  
        db, err := get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.String(http.StatusOK,"?? open db failed:"+err.Error())
            return
        }
        title := c.PostForm("title")
        color := c.PostForm("color")
        shelf_id := c.PostForm("shelf_id")
        tag,err:=new_article(db,title ,color ,shelf_id )
        if err !=nil{
            c.String(http.StatusOK,"?? error"+err.Error())
            return
        }
        c.String(http.StatusOK,"!!"+tag)

    });

    r.POST("/edit_article",func(c *gin.Context){  
        db, err := get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.String(http.StatusOK,"?? open db failed:"+err.Error())
            return
        }
        tag :=  c.PostForm("tag")
        title := c.PostForm("title")
        color := c.PostForm("color")
        shelf_id := c.PostForm("shelf_id")
        _,err=update_article(db,tag,title,color,shelf_id)
        if err !=nil{
            c.String(http.StatusOK,"?? error"+err.Error())
            return
        }
        c.String(http.StatusOK,"!!"+tag)
    });

    r.POST("/del_article",func(c *gin.Context){ 
        tag := c.PostForm("tag")
        db, err := get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.String(http.StatusOK,"?? open db failed:"+err.Error())
            return
        }

        _,err=del_article(db,tag,db_folder)
        if err!=nil{
            fmt.Println("?? error deleting article:"+err.Error())
            c.String(http.StatusOK,"?? error deleting article:"+err.Error())
        }
        c.String(http.StatusOK,"!!"+tag)
    });

    r.GET("/articles/:page",func(c *gin.Context){ 
        db, err := get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }

        page,err :=strconv.Atoi(c.Param("page"))
        if err!=nil{
            page =1
        }
        page_len := get_article_list_len(db)
        articles,err:=list_articles(db,page,page_len)
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
            return
        }
        cnt,_ :=count_articles(db)
        page_count := calc_pages(cnt, page_len)
        c.HTML(http.StatusOK,"articles.html",gin.H{
            "articles":articles,
            "page_bar":draw_page_bar(page_count,page,"background-color:#1E9FFF","/list_image/"),
            "wrap_class":get_page_wrap_class(db,host_name),
        });
    })

    r.GET("/show_article/:tag",func(c *gin.Context){ 
        tag := c.Param("tag")
        db, err := get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        
        article,err :=get_article_record(db,tag)
        if err !=nil{
            if  err.Error()=="no record"{
                c.Redirect(http.StatusTemporaryRedirect,"/error/2")
            }else{
                c.Redirect(http.StatusTemporaryRedirect,"/error/1")
            }            
            return
        }

        pages,err := list_article_pages(db,tag,db_folder)
        if err !=nil{
            if  err.Error()=="no record"{
                c.Redirect(http.StatusTemporaryRedirect,"/error/2")
            }else{
                c.Redirect(http.StatusTemporaryRedirect,"/error/1")
            }            
            return
        }
        
        c.HTML(http.StatusOK,"show_article.html",gin.H{
            "article":article,
            "article_tag":article.Tag,
            "pages":pages,
            "wrap_class":get_page_wrap_class(db,host_name),
        });

    });

    r.GET("/show_article_sort/:tag",func(c *gin.Context){ 
        tag := c.Param("tag")
        db, err := get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        
        article,err :=get_article_record(db,tag)
        if err !=nil{
            if  err.Error()=="no record"{
                c.Redirect(http.StatusTemporaryRedirect,"/error/2")
            }else{
                c.Redirect(http.StatusTemporaryRedirect,"/error/1")
            }            
            return
        }

        pages,err := list_article_pages(db,tag,db_folder)
        if err !=nil{
            if  err.Error()=="no record"{
                c.Redirect(http.StatusTemporaryRedirect,"/error/2")
            }else{
                c.Redirect(http.StatusTemporaryRedirect,"/error/1")
            }            
            return
        }
        max_len := 200
        for i:=0;i<len(pages);i++{
            pages[i].Data=html_shrink(pages[i].Data,max_len)
        }
        
        c.HTML(http.StatusOK,"article_page_sort.html",gin.H{
            "article":article,
            "article_tag":article.Tag,
            "pages":pages,
            "wrap_class":get_page_wrap_class(db,host_name),
        });
    });

    r.POST("/article_page_sort",func(c *gin.Context){ 
        db, err := get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.String(http.StatusOK,"?? open db failed")
        }
        order_arr :=strings.Split(c.PostForm("order_str"),";")
        for _,str:=range(order_arr){
            if !strings.Contains(str,":"){
                continue
            }
            pair:=strings.Split(str,":")
            _,err=article_page_set_order(db, pair[0],pair[1])
        }
        if err !=nil{
            c.String(http.StatusOK,"?? error occured")
            return
        }
        c.String(http.StatusOK,"!!Done")
    })

    r.POST("/search_article",func(c *gin.Context){ 
        db, err := get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }

        target:=c.PostForm("target")
        articles,err :=search_article(db,target)
        if err!=nil{
            fmt.Println("?? error searching article:",err.Error())
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        c.HTML(http.StatusOK,"articles.html",gin.H{
            "articles":articles,
            "page_bar":"",
            "wrap_class":get_page_wrap_class(db,host_name),
        });
    })

    // handle article pages
    r.GET("/article_page/:tags",func(c *gin.Context){ 
        db, err := get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }

        tags :=strings.Split(c.Param("tags"),"_")
        tag :=tags[0]
        pg_tag :=tags[1]
        content :=""
        if pg_tag !=""{
            if err !=nil{
               fmt.Println("?? error getting article page")
            }
            _,old_note,err:=get_text(db, db_folder ,pg_tag )
            if err==nil{
                content=old_note
            }
        }
        article,err :=get_article_record(db,tag)
        c.HTML(http.StatusOK,"article_page.html",gin.H{
            "tag":tag,
            "pg_tag":pg_tag,
            "title":article.Title,
            "content":content,
            "wrap_class":get_page_wrap_class(db,host_name),
        });

    });
    
    r.POST("/del_article_page",func(c *gin.Context){
        db, err := get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.String(http.StatusOK,"?? open db failed")
        }

        pg_tag :=c.PostForm("pg_tag")
        _,err=del_article_page(db,pg_tag,db_folder)
        if err!=nil{
            c.String(http.StatusOK,"?? error msg:"+err.Error())
            return
        }
        c.String(http.StatusOK,"!!"+pg_tag)

    });

    r.POST("/article_page_add",func(c *gin.Context){
        db, err := get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.String(http.StatusOK,"?? error open the database file")
            return
        }

        tag:=c.PostForm("tag")
        content :=c.PostForm("content") 
        pg_tag,err:=add_article_page(db,tag,content,db_folder)
        if err !=nil{
            fmt.Println("?? error adding page:",err.Error())
            c.String(http.StatusOK,"?? error:"+err.Error())
            return
        }
        c.String(http.StatusOK,"!!"+pg_tag)
        
    })

    r.POST("/article_page_update",func(c *gin.Context){
        db, err := get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.String(http.StatusOK,"?? error open the database file")
            return
        }

        pg_tag:=c.PostForm("pg_tag")
        content :=c.PostForm("content")
        _,err =edit_article_page(db,pg_tag,content,db_folder)
        if err !=nil{
            fmt.Println("?? error updating page:",err.Error())
            c.String(http.StatusOK,"?? error updating page:"+err.Error())
            return
        }
        c.String(http.StatusOK,"!!"+pg_tag)

    });

    // ================= FILE OPEN =========================
    r.GET("/show/:dev_ino",func(c *gin.Context){  
        db, err := get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        var device_id uint64
        var ino uint64
        var url string

        if strings.Contains(c.Param("dev_ino"),"_"){
            device_id,ino,err:=dev_ino_uint64(c.Param("dev_ino"))
            if err !=nil{
                c.String(http.StatusOK,"??query error")
                return
            }
            url,err = file_url(db,device_id,ino,100,"/")
            if err !=nil{
                c.String(http.StatusOK,"??error, getting file_url failed")
                return
            }
            
        }else{
            // by tag, tag is from file_note table
            if len(c.Param("dev_ino")) != 10{
                c.String(http.StatusOK,"??error,query format problem")
                return 
            }
            tag :=c.Param("dev_ino")
            note,err:=get_note_by_tag(db,tag)
            if err!=nil{
                c.String(http.StatusOK,"??error,getting note failed")
                return
            }
            url= root_dir+note.File_dir+note.File_name
        }
        fnode,err :=get_Fnode(url,false)
        if err !=nil{
            c.String(http.StatusOK,"??error,getting note url failed:"+err.Error())
            return
        }
        device_id =uint64(fnode.Dev)
        ino = fnode.Ino

        file_ext :=file_suffix(url)
        file_name :=path_file_name(url,sys_delim())
        opener :=get_host_opener(db,file_ext)
        if opener=="browser"{
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
            return
        }
        if opener !=""{
            // prioritize settings in the db            
            fnode,err :=query_fnode(db,device_id,ino)        
            cmd := exec.Command(opener,url)
            err = cmd.Start()
            if err !=nil{
                c.String(http.StatusOK,"??error")
            }
            folder_dev_ino := strconv.FormatUint(device_id,10)+"_"+strconv.FormatUint(fnode.Parent_ino,10)
            active_dev_ino := strconv.FormatUint(device_id,10)+"_"+strconv.FormatUint(ino,10)
            fmt.Println("redirect->"+"/list/"+strconv.FormatUint(device_id,10)+"_"+strconv.FormatUint(fnode.Parent_ino,10))
            // http.StatusMovedPermanently not suitable
            c.Redirect(http.StatusTemporaryRedirect,"/list/"+folder_dev_ino+"&"+active_dev_ino)
            return
        }

        switch file_ext{
            
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
                "file_name":file_name,
                "wrap_class":get_page_wrap_class(db,host_name),
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
        defer db.Close()
        if err !=nil{
            c.String(http.StatusOK,"?? open db error")
        }
        delim:=sys_delim()
        device_id,ino,err:=dev_ino_uint64(c.Param("ino"))
        if err!=nil{
            c.String(http.StatusOK,"??query error")
        }else{
            url,err := file_url(db,device_id,ino,100,delim)
            if err!=nil{
                c.String(http.StatusOK,"??db error")
            }
            
            rel_url := relative_path_of(url,root_dir)
            file_name:= path_file_name(rel_url,delim)
            file_dir:= path_dir_name(rel_url,delim)
            sc_type :="f"
            if strings.HasSuffix(url,delim){
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
        delim := sys_delim()
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.String(http.StatusOK,"?? open db error")
            return
        }
        
        device_id,ino,err:=dev_ino_uint64(c.Param("ino"))
        if err!=nil{
            c.String(http.StatusOK,"??query error")
        }else{
            url,err :=file_url(db,device_id,ino,100,delim)
            if err!=nil{
                c.String(http.StatusOK,"??db error")
            }
            rel_url := relative_path_of(url,root_dir)
            file_name:= path_file_name(rel_url,delim)
            file_dir:= path_dir_name(rel_url,delim)
            sc_type  := "f"
            if file_name == ""{
                sc_type  = "d"
            }
            ok,err:=del_shortcut(db,file_dir,file_name,sc_type)
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

    r.GET("/del_shortcut_id/:scid",func(c *gin.Context){
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.String(http.StatusOK,"?? open db error")
            return
        }

        _,err:=del_shortcut_id(db,c.Param("scid"))
        if err !=nil{
            c.String(http.StatusOK,"??error")
            return
        }
        c.String(http.StatusOK,"!!done")
    })

    r.GET("/stash/:ino",func(c *gin.Context){
        db, err = get_db(db_file)
        delim:=sys_delim()
        defer db.Close()
        if err !=nil{
            c.String(http.StatusOK,"?? open db error")
            return
        }
 
        device_id,ino,err:=dev_ino_uint64(c.Param("ino"))
        if err!=nil{
            c.String(http.StatusOK,"??query error")
            return
        }

        url,err := file_url(db,device_id,ino,100, delim)
        if err!=nil{
            c.String(http.StatusOK,"??db error")
            return
        }
        if url == root_dir{
            c.String(http.StatusOK,"??root_dir stash is not allowed")
            return
        }
        rel_url := relative_path_of(url,root_dir)
        file_name:= path_file_name(rel_url, delim)
        file_dir:= path_dir_name(rel_url, delim)
        sc_type :="s"  // file stash
        if strings.HasSuffix(url, delim){
            sc_type="t" // folder stash
        }
        ok,err:=add_shortcut(db,file_dir,file_name,sc_type)
        if err!=nil{
            c.String(http.StatusOK,"??not done")
            return
        }else{
            if ok{
                c.String(http.StatusOK,"!!done")
                return
            }else{
                c.String(http.StatusOK,"??existing")
                return
            }
        }   
    });

    r.GET("/unstash/:ino",func(c *gin.Context){
        delim := sys_delim()
        db, err = get_db(db_file)
        defer db.Close()

        if err !=nil{
            c.String(http.StatusOK,"?? open db error")
            return
        }

        device_id,ino,err:=dev_ino_uint64(c.Param("ino"))
        if err!=nil{
            c.String(http.StatusOK,"??query error")
        }else{
            url,err :=file_url(db,device_id,ino,100,delim)
            if err!=nil{
                c.String(http.StatusOK,"??db error")
            }
            rel_url := relative_path_of(url,root_dir)
            file_name:= path_file_name(rel_url,delim)
            file_dir:= path_dir_name(rel_url,delim)
            sc_type  := "s"
            if file_name == ""{
                sc_type  = "t"
            }
            ok,err:=del_shortcut(db,file_dir,file_name,sc_type)
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

    r.GET("/manange_shortcut",func(c *gin.Context){
        db, err = get_db(db_file)
        defer db.Close()
        if err!=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        pin_files,err := shortcut_list(db,"f")
        if err !=nil && err.Error() !="no record"{
            fmt.Printf("error:%q\n",err)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
            return 
        }
        pin_folders,err :=shortcut_list(db,"d")
        if err !=nil && err.Error() !="no record"{
            fmt.Printf("error:%q\n",err)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
            return 
        }
        stash_files,err :=shortcut_list(db,"s")
        if err !=nil && err.Error() !="no record"{
            fmt.Printf("error:%q\n",err)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
            return 
        }
        stash_folders,err :=shortcut_list(db,"t")
        if err !=nil && err.Error() !="no record"{
            fmt.Printf("error:%q\n",err)
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
            return 
        }

        c.HTML(http.StatusOK,"shortcuts.html",gin.H{
            "pin_files":pin_files,
            "pin_folders":pin_folders,
            "stash_files":stash_files,
            "stash_folders":stash_folders,
            "wrap_class":get_page_wrap_class(db,host_name),
        })

    });

    r.GET("/put/:ino",func(c *gin.Context){
        // delim := sys_delim()
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }

        device_id,ino ,err := dev_ino_uint64(c.Param("ino"))
        if err!=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
            return 
        }
        url,err  := file_url(db, device_id,ino,100,sys_delim())
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
            return 
        }
        list_file,err := shortcut_list(db,"s") 
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
            return 
        }
        for i:=0;i<len(list_file);i++{
            list_file[i].File_dir=str_native_delim(root_dir+list_file[i].File_dir)
        }

        list_folder,err :=shortcut_list(db,"t")
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
            return 
        }
        for i:=0;i<len(list_folder);i++{
            list_folder[i].File_dir=str_native_delim(root_dir+list_folder[i].File_dir)
        }

        c.HTML(http.StatusOK,"put.html",gin.H{
            "list_file":list_file,
            "list_folder":list_folder,
            "root_dir":root_dir,
            "url":url,
            "dev_ino":c.Param("ino"),
            "wrap_class":get_page_wrap_class(db,host_name),
        })

    });
    r.POST("/putdown",func(c *gin.Context){
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.String(http.StatusOK,"?? open db error")
            return
        }

        dev_ino:=c.PostForm("dev_ino")
        scid := c.PostForm("scid")
        _,err:=stash_putdown(db,scid, dev_ino,root_dir)
        if err !=nil{
            fmt.Println("??error:"+err.Error())
            c.String(http.StatusOK,"?? error"+err.Error())
            return
        }
        c.String(http.StatusOK,"!!done")
    });

    r.GET("/rebuild",func(c *gin.Context){
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.String(http.StatusOK,"?? open db error")
            return
        }
        
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
        // c.Redirect(http.StatusTemporaryRedirect,"/error/101")
        db, err = get_db(db_file)
        host_name := get_host_name()
        defer db.Close()
        if err !=nil{
            c.String(http.StatusOK,"?? open db error")
            return
        }

        openers,err := enum_host_openers(db,host_name)
        if err!=nil{
            openers=""
        }

        c.HTML(http.StatusOK,"settings.html",gin.H{            
            "openers":openers,
            "wrap_class":get_page_wrap_class(db,host_name),
            "img_page_len":strconv.Itoa(get_img_page_len(db)),
            "notes_page_len":strconv.Itoa(get_notes_page_len(db)),
            "article_list_len":strconv.Itoa(get_article_list_len(db)),
        });

    });
    r.POST("/settings",func(c *gin.Context){
        // c.Redirect(http.StatusTemporaryRedirect,"/error/101")
        db, err = get_db(db_file)
        host_name := get_host_name()
        defer db.Close()
        if err !=nil{
            c.String(http.StatusOK,"?? open db error")
            return
        }

        set_page_wrap_class(db,host_name,c.PostForm("wrap_class"))
        set_img_page_len(db,c.PostForm("img_page_len"))
        set_notes_page_len(db,c.PostForm("notes_page_len"))
        set_article_list_len(db,c.PostForm("article_list_len"))
        // LIST TO UPDATE
        reg:=regexp.MustCompile(`\s*([\w\d]+)\s*=\s*(\S.*)\s*[\r\n]`)
        opener_list := reg.FindAllStringSubmatch(c.PostForm("openers"),-1)
        for _,opener:=range(opener_list){
            set_host_opener(db,host_name,opener[1],opener[2])
        }
        // LIST TO CLEAR
        reg=regexp.MustCompile(`\s*([\w\d]+)\s*=\s*[\r\n]`)
        opener_list = reg.FindAllStringSubmatch(c.PostForm("openers"),-1)
        for _,opener:=range(opener_list){
            clear_setting(db,opener[1]+"_opener", host_name)
        }

        c.String(http.StatusOK,"!!Done")
    });


    r.GET("/gallery/:ino",func(c *gin.Context){
        db, err = get_db(db_file)
        defer db.Close()
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }

        device_id,ino,err:=dev_ino_uint64(c.Param("ino"))
        url,err :=file_url(db,device_id,ino,100,"/")
        if err !=nil{
            c.Redirect(http.StatusTemporaryRedirect,"/error/1")
        }
        var image_list []string
        children_fnodes := folder_entries(url)
        for _,child :=range(children_fnodes){
            ext_name :=strings.ToLower(file_suffix(child.Name))
            ext_set :=make_set([]string{"png","gif","jpeg","jpg","bmp","webp","svg"})
            if ext_set.Has(ext_name){
                image_list =append(image_list,"/show/"+child.device_id()+"_"+child.ino())
            }
        }

        c.HTML(http.StatusOK,"gallery.html",gin.H{
            "image_list":image_list,
            "dev_ino":c.Param("ino"),
            "wrap_class":get_page_wrap_class(db,host_name),
        });
        
    });

    // r.GET("/settings",func(c *gin.Context){
    //     settings,err:=fetch_settings()
    //     c.HTML(http.StatusOK,"settings.html",gin.H{
    //         "settings":settings,
    //     });
    // })


    r.GET("/error/:no",func(c *gin.Context){
        db, err = get_db(db_file)
        defer db.Close()

        if err !=nil{
            c.HTML(http.StatusOK,"error.html",gin.H{
                "error_msg":"fatal error failed to open db",
            })
            return
        }
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
        case "9":
            c.HTML(http.StatusOK,"error.html",gin.H{
                "error_msg":"the root_dir did not match the one in the database, please check and restart, or you can try <a href='/rebuild'>rebuild the cache</a>",
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
