var Color_coden={"green":1,"red":2,"blue":3,"purple":4,"orange":5,"yellow":6,"grey":7};
function get_color_code(color){
    //color=$("#color_tag").attr("class").split("_")[1];
    if(Color_coden[color]==undefined){
        return 0;
    }else{
        return Color_coden[color];
    }
}

function get_color_by_code(c){
    for (color in Color_coden){
        if (Color_coden[color] == c){
            return color;
        }        
    }
    return "green"; // default
}


layui.use(['dropdown', 'util', 'layer'], function(){
    var dropdown = layui.dropdown,
            util = layui.util,
            layer = layui.layer,
                $ = layui.jquery;

    // for color option
    dropdown.render({
        elem: '#color_menu',
        data: [
            {  title: '<img class="color_green_dot" id="color_tag_1" src="/public/css/blank.png">',
                id: get_color_code("green")
            },{  
                title: '<img class="color_red_dot" id="color_tag_2" src="/public/css/blank.png">',
                id: get_color_code("red")
            },{
                title: '<img class="color_blue_dot" id="color_tag_3" src="/public/css/blank.png">',
                id: get_color_code("blue")
            },{
                title: '<img class="color_purple_dot" id="color_tag_4" src="/public/css/blank.png">',
                id: get_color_code("purple")
            },{
                title: '<img class="color_orange_dot" id="color_tag_5" src="/public/css/blank.png">',
                id: get_color_code("orange")
            },{
                title: '<img class="color_yellow_dot" id="color_tag_6" src="/public/css/blank.png">',
                id: get_color_code("yellow")
            },{
                title: '<img class="color_grey_dot" id="color_tag_7" src="/public/css/blank.png">',
                id: get_color_code("grey")
            }
        ],
        click: function(obj){
            color = get_color_by_code(obj.id);
            // $("#color_tag").html(obj.title);
            $("#color_tag").removeClass().addClass("color_"+color+"_dot");
            //layer.tips('点击了：'+ obj.title, this.elem, {tips: [1, '#5FB878']})
        }
    });

        //for file options
    dropdown.render({
        elem: '.file_option',
        trigger: 'mouseenter',
        delay:1500,
        data: [
            {title: '<span>Add/Edit</span>',    id: "add"},
            {title: '<span>Del</span>',    id: "del"},
            {title: '<span>Rename</span>', id: "rename"},
            {title: '<span>Pin/Unpin</span>', id: "pin"}],
        click: function(data, othis){
            if(data.id=="add"){
                AddNote($(this.elem).attr("value") );
            }else if (data.id=="del"){
                if (confirm("Your are DELETING this note, ARE YOU SURE?") ){
                    //window.location.replace("/del_note/"+$(this.elem).attr("value"));
                    DelNote($(this.elem).attr("value"));
                }
            }else if (data.id=="rename"){
                Rename($(this.elem).attr("value"));
            }else if (data.id=="pin"){
                toggle_shortcut_file($(this.elem).attr("value"));
            }

            //event.stopPropagation();
            // how to stop this event propagation ?
            // $(this.elem).click(function(event) {
            //     event.stopPropagation();
            // });            
        }
    });
    

    // for workspace
    dropdown.render({
        elem: '#btn_workspace',
        data: 
        [   
            { title: 'Folders',
                isSpreadItem: true,
            // id: 102,
                type: 'group',  //菜单类型，支持：normal/group/parent/-
            //   child: [{
            //    title: 'File1',
            //          id: 103},
            //          {title: 'File2',
            //            id: 104
            //           }] 
            //child:<%= @shortcut.master_folders_to_str %>
            },


            //{ title: 'menu item 2',
            //  id: 101,
            //  href: 'https://www.layui.com/',
            //  target: '_blank' },
            { type: '-'},
            { title: 'Files',
                isSpreadItem: false,
                id: 102,
                type: 'parent',
            //    child:<%= @shortcut.master_files_to_str %> 
            },
            { type: '-'},
            { 
                title: 'Manage',              
                href: '/manange_shortcut'
            }
        ]
                
    });

    $("#toggle_view").click(function(){
        if ($("#toggle_view").attr("value")=="0"){
            $('.note_visible').addClass("layui-show"); //.layui-colla-content 
            $("#toggle_view").attr("value","1");
            $("#toggle_view").text("收起");
        }else{ 
            $('.note_visible').removeClass("layui-show");
            $("#toggle_view").attr("value","0");
            $("#toggle_view").text("展开");
        }
    });
});

// utility functions
function FileNamePostfix(file_name){
    if (file_name.match(/\./)){
        _do_index = file_name.lastIndexOf(".");
        return file_name.substring(_do_index + 1); // 到最后一个字符
    }else{
        return file_name;
    }
}

function FileNamePrefix(file_name){
    if (file_name.match(/\./)){
        _do_index = file_name.lastIndexOf(".");
        return file_name.substring(0, _do_index); // 到最后一个字符
    }else{
        return "";
    }
}

// for dialog showing
function show_dialog(id,wide){
    $(id).css("position","fixed");
    if(wide){
        $(id).css({'top':window.innerHeight/10,'left':window.innerWidth/10});
    }else{
        $(id).css({'top':window.innerHeight/3,'left':window.innerWidth/2-200});
    }    
    $(id).css("background-color",'white');
    $(id).draggable();
    $(id+' .buttonCancel').click(function(){
        $(id).hide(100);
    });
    $(id+' .close2').click(function(){
        $(id).hide(100);
    });
}

function AddNote(ino_id){
    // display the dialog box
    show_dialog("#add_note_dialog",true);
    $(".tox-tinymce").height($("#add_note_dialog").height()-100);
    if ($('#dialog_ino_id').val()!=ino_id){
        $('#dialog_ino_id').val(ino_id);
        //$('#note_content').append($.trim($("#item_"+ino_id).html()) );
        if ($.trim($("#item_"+ino_id).html() !="")){
            tinyMCE.get('note_content').setContent($.trim($("#item_"+ino_id).html() ));
            $("#dialog_md5_digest").val(hex_md5(tinyMCE.get('note_content').getContent()));
        }else{
            tinyMCE.get('note_content').setContent("");
        }
    } 
    
    // set dialog title
    $("#dialog_title").html($.trim($("#filename_"+ino_id).html()) )
    $("#add_note_dialog").show(100);  
    
    //define the submit button actions  
    $('#submit_add').unbind("click").click(function(){  
        PostNote();
        $("#add_note_dialog").hide(100);
        event.preventDefault();	//阻滞二次提交
    });

    $('#save').unbind("click").click(function(){  
        if (PostNote() ){
            alert("done");
        }else{
            alert("not saved!");
        }
        event.preventDefault();	//阻滞二次提交
    });

    
}

function PostNote(){
    color_code = get_color_code($("#color_tag").attr("class").split("_")[1]);
    ino_id= $('#dialog_ino_id').attr("value");
    item_value =$("#item_"+ino_id).attr("value");
    var act="";
    var act_target="";

    if(item_value ==""){
        //add note
        act ="add_note";
        act_target = ino_id;
    }else{
        //edit note
        act ="edit_note";
        act_target = item_value;
    }
    var md5_digest_new=hex_md5(tinyMCE.get('note_content').getContent());
    var md5_digest_old=$("#dialog_md5_digest").val();
    if (md5_digest_new==md5_digest_old){
        // alert("nothing changed since last save!");
    }else{
        $.post("/"+act+"/"+act_target,{'ino_id' : ino_id,'tag': item_value, 'note':tinyMCE.get('note_content').getContent(),'color': color_code},function(data,status){
            if(status=="success" && data.match(/^\!\!(\w+)/)){
                $("#item_"+ino_id).html(tinyMCE.get('note_content').getContent());
                $("#item_"+ino_id).parent().addClass("note_visible");
                $("#item_"+ino_id).attr("value",data.substr(2));
                img_str='<img class="color_'+ get_color_by_code(color_code)+'_dot" src="/public/css/blank.png">'
                $("#item_color_"+ino_id).html(img_str);
                $("#dialog_md5_digest").val(md5_digest_new);
                //tinyMCE.activeEditor.setContent('');
            }else{
                alert("Add Note note failed"+data.substr(2));  
                return false                          
            }
        });
    }
    return true;
}

function Rename(ino_id){
    show_dialog("#rename_dialog",false);
    var old_name  =$.trim($("#filename_"+ino_id).html());
    if ($('#dialog_rename_ino_id').val()!=ino_id){        
        $('#new_name').val(FileNamePrefix(old_name) );
        $('#new_name_ext').val(FileNamePostfix(old_name) );
        $('#dialog_rename_ino_id').val(ino_id);
    }
    $("#dialog_rename_title").html("Change the old name" );
    $("#rename_dialog").show(100);    
    $('#submit_rename').unbind("click").click(function(){       
        $.post("/rename/"+ino_id,{
                'ino_id' : ino_id, 'new_name':$('#new_name').val(),
                    'new_name_ext':$('#new_name_ext').val()},function(data,status){
            if(status=="success" && data.match(/^\!\!/)){
                //console.log(data);
                tt=data.split(/:/);
                //console.log(tt);
                new_name=tt[tt.length-1];
                //$('#new_name').val()+"."+$('#new_name_ext').val()
                $("#filename_"+ino_id).html(new_name);  
            }else{
                //
                //console.log(data);
                alert("Note modified"+data.substr(2));                
            }
        });	
        event.preventDefault();	//阻滞二次提交
        $("#rename_dialog").hide(100);
    });
}


function DelNote(id){
    $.get("/del_note/"+id,function(data,status){
        if(status=="success" && data.match(/^\!\!(\w+)/)   ){
            matched_data=data.match(/^\!\!(\w+)/);
            id=matched_data[1];
            $("#item_color_"+id+" img").attr("class","color_default_dot");
            $("#item_"+id).html("");
        }else{
            alert("failed:"+data);
        }
    });
}

// for tinymce
tinymce.init({
    selector: '#note_content',
    language:'zh_CN',
    plugins: 'importcss print preview searchreplace autolink directionality visualblocks visualchars fullscreen image link  template code codesample table charmap hr pagebreak nonbreaking anchor insertdatetime advlist lists wordcount imagetools textpattern paste emoticons autosave ',
    toolbar: 'code undo redo | formatselect styleselect forecolor backcolor image  bold italic underlineremoveformat |\
    blockquote subscript superscript  | alignleft aligncenter alignright  lineheight | \
    strikethrough link  fontselect fontsizeselect bullist numlist | \
    table  charmap hr pagebreak insertdatetime | fullscreen ',
    fontsize_formats: '12px 14px 16px 18px 24px 36px 48px 56px 72px',
    autosave_ask_before_unload: true,
    //icons:'ax-color',
    height:350,
    content_css: "/public/css/editor.css",
    images_upload_url: '/image_upload'

    // images_upload_handler: function (blobInfo, success, failure, progress) {
    //     var xhr, formData;
    //     xhr = new XMLHttpRequest();
    //     xhr.withCredentials = false;
    //     xhr.open('POST', '/image_upload');

    //     xhr.upload.onprogress = function(e){
    //         progress(e.loaded / e.total * 100);
    //     }

    //     xhr.onload = function() {
    //         var json;
    //         if (xhr.status == 403) {
    //             failure('HTTP Error: ' + xhr.status, { remove: true });
    //             return;
    //         }
    //         if (xhr.status < 200 || xhr.status >= 300 ) {
    //             failure('HTTP Error: ' + xhr.status);
    //             return;
    //         }
    //         json = JSON.parse(xhr.responseText);
    //         if (!json || typeof json.location != 'string') {
    //             failure('Invalid JSON: ' + xhr.responseText);
    //             return;
    //         }
    //         success(json.location);
    //     };

    //     xhr.onerror = function () {
    //         failure('Image upload failed due to a XHR Transport error. Code: ' + xhr.status);
    //     }

    //     formData = new FormData();
    //     formData.append('file', blobInfo.blob(), blobInfo.filename());
    
    //     xhr.send(formData);

    // }

    
});
    
// for shortcut making
function toggle_shortcut(id,is_add){
    $("#shortcut_status").attr("value","0"); // reset
    var target_url="";
    if (is_add){
        target_url="/add_shortcut/"+id;
    }else{
        target_url="/del_shortcut/"+id;
    }
    $.get(target_url,{},function(data,status){
        if(status=="success" && data.match(/^\!\!/)){
            $("#shortcut_status").attr("value","1");
            // alert("done");
            
        }else{
            $("#shortcut_status").attr("value","0");
            //alert("shortcut not added:"+data);         
        }
    });    
}

function toggle_shortcut_file(id){
    var is_add;
    if ($("#pin_"+id).attr("value")=="true"){
        is_add=true;
    }else{
        is_add=false;
    }

    toggle_shortcut(id,is_add);
    setTimeout(function(){
        if ($("#shortcut_status").attr("value")=='1'){    
            if (is_add){
                // add the pin logo
                $("#pin_"+id).removeClass("unpinned");
                $("#pin_"+id).addClass("pinned");
                $("#pin_"+id).attr("value","false");
            }else{
                // remove the pin logo
                $("#pin_"+id).removeClass("pinned");
                $("#pin_"+id).addClass("unpinned");
                $("#pin_"+id).attr("value","true");
            }
        }
    },100);

}

function toggle_shortcut_folder(id){
    var is_add;
    if ($("#add_shortcut").attr("value")=="true"){
        is_add=true;
    }else{
        is_add=false;
    }
    toggle_shortcut(id,is_add);
    setTimeout(function(){
        if ($("#shortcut_status").attr("value")=='1'){    
            if (is_add){
                // add the pin logo
                $("#shortcut_icon_"+id).removeClass("layui-icon-rate");
                $("#shortcut_icon_"+id).addClass("layui-icon-rate-solid");
                $("#add_shortcut").attr("value","false");
            }else{
                // remove the pin logo
                $("#shortcut_icon_"+id).removeClass("layui-icon-rate-solid");
                $("#shortcut_icon_"+id).addClass("layui-icon-rate");
                $("#add_shortcut").attr("value","true");
            }
        }
    },100);
}