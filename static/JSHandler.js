function uploadFile(){
    var input = document.createElement('input');
    input.type = 'file';

        input.onchange = e => { 

        // getting a hold of the file reference
        var file = e.target.files[0];
        var fileName = file.name
        sendFile(fileName)
        // setting up the reader
        //var reader = new FileReader();
        //reader.readAsText(file,'UTF-8');

        // here we tell the reader what to do when it's done reading...
        //reader.onload = readerEvent => {
        //    var content = readerEvent.target.result; // this is the content!
        //    sendFile(content)
        //    console.log(fileName);
        //}

    }
    input.click();
}
function sendFile(fileName){
    console.log(fileName)
    $.ajax({
        data: {"fileName":fileName },
        type: "POST",
        dataType: "json",
        url: "http://localhost:8080/fileUpload",
    })
    .done(function( data, textStatus, jqXHR ) {
		//console.log(selectedNode)
    })
    .fail(function( jqXHR, textStatus, errorThrown ) {
        console.log( "Fail " +  textStatus);
    });
}

/*function uploadFile2(elemId) {
    var elem = document.getElementById(elemId);
    if(elem && document.createEvent) {
       var evt = document.createEvent("MouseEvents");
       evt.initEvent("click", true, false);
       elem.dispatchEvent(evt);
    }
 }*/

$(document).ready(function() {
    $("#selectOrigin").on("change", function() {
        console.log($(this).val())
        if ($(this).val() === "None") {
            $("#label1").hide();
        }
        else {
            $("#label1").show();
        }
    });
});
$(document).ready(function() {
    $("#selectOriginForFileRequest").on("change", function() {
        console.log($(this).val())
        if ($(this).val() === "None") {
            $("#label2").hide();
        }
        else {
            $("#label2").show();
        }
    });
});
function getID(){
    $.ajax({
        type: "GET",
        dataType: "json",
        url: "http://localhost:8080/id",
    })
    .done(function( data, textStatus, jqXHR ) {
        document.getElementById("OwnID").innerHTML = "<strong>Gossiper Name: </strong>"+data
    })
    .fail(function( jqXHR, textStatus, errorThrown ) {
        console.log( "Fail: " +  textStatus);
    });
}
function requestFile(){
    selectedOrigin = $('#selectOriginForFileRequest').find(":selected").text();
    requestedFileHash = document.getElementById("requestFileHash").value
    fileName = document.getElementById("fileName").value
    document.getElementById('requestFileHash').value = ''
    document.getElementById('fileName').value = ''
    //console.log(newNode)
    $.ajax({
        data: {"selectedOrigin" : selectedOrigin, "requestedFileHash": requestedFileHash,"fileName" :fileName},
        type: "POST",
        dataType: "json",
        url: "http://localhost:8080/requestFile",
    })
    .done(function( data, textStatus, jqXHR ) {
		//console.log(selectedNode)
    })
    .fail(function( jqXHR, textStatus, errorThrown ) {
        console.log( "Fail " +  textStatus);
    });
}
function getMessagesList(){
    $.ajax({
        data: {},
        type: "GET",
        dataType: "json",
        url: "http://localhost:8080/messages",
    })
    .done(function( data, textStatus, jqXHR ) {
        var arrayMenssages = data.map(htmlMessages)
        var stringToHTML = arrayMenssages.join(" ")
        document.getElementById("ChatBox").innerHTML = stringToHTML
    })
    .fail(function( jqXHR, textStatus, errorThrown ) {
        console.log( "Fail " +  textStatus);
    });
}
function getNodesList(){
    $.ajax({
        type: "GET",
        dataType: "json",
        url: "http://localhost:8080/node",
    })
    .done(function( data, textStatus, jqXHR ) {
        var arrayNodes = data.map(htmlKnowNodes)
        var stringToHTML = arrayNodes.join(" ")
        document.getElementById("NodeBox").innerHTML = stringToHTML
    })
    .fail(function( jqXHR, textStatus, errorThrown ) {
        console.log( "Fail " +  textStatus);
    });
}
function getOriginsList(){
    $.ajax({
        type: "GET",
        dataType: "json",
        url: "http://localhost:8080/privateMessage",
    })
    .done(function( data, textStatus, jqXHR ) {
        $.each(data, function(i, p) {
            var exists = $("#selectOrigin option")
               .filter(function (i, o) { return o.value === p; })
               .length > 0;
            //console.log($(exists))
            if(!exists){
                $('#selectOrigin').append($('<option></option>').val(p).html(p));
            }
            //Now for the other selector
            var exists = $("#selectOriginForFileRequest option")
            .filter(function (i, o) { return o.value === p; })
            .length > 0;

         if(!exists){
             $('#selectOriginForFileRequest').append($('<option></option>').val(p).html(p));
         }
        });
    })
    .fail(function( jqXHR, textStatus, errorThrown ) {
        console.log( "Fail " +  textStatus);
    });
}
function sendPrivateMessage(){
    selectedOrigin = $('#selectOrigin').find(":selected").text();
    messageString = document.getElementById("privateMessageString").value
    document.getElementById('privateMessageString').value = ''
    //console.log(newNode)
    $.ajax({
        data: {"selectedOrigin" : selectedOrigin, "privateMessageString": messageString},
        type: "POST",
        dataType: "json",
        url: "http://localhost:8080/privateMessage",
    })
    .done(function( data, textStatus, jqXHR ) {
		//console.log(selectedNode)
    })
    .fail(function( jqXHR, textStatus, errorThrown ) {
        console.log( "Fail " +  textStatus);
    });
}
function getEverything(){
    getID()
    getMessagesList()
    getNodesList()
    getOriginsList()
}
function getMessagesAndNodesAndOrigins(){
    getMessagesList()
    getNodesList()
    getOriginsList()
}
$(document).ready(function () {
    getEverything()
});
setInterval(() => {getMessagesAndNodesAndOrigins()}, 1000)
function htmlMessages(message){
    var messageTreated ="<li>"+message +"</li>"
    return messageTreated
}

function htmlKnowNodes(node){
    var nodeTreated ="<li>"+node +"</li>"
    return nodeTreated
}

function addNode(){
    newNode = document.getElementById("nodeToAdd").value
    //console.log(newNode)
    $.ajax({
        data: {"nodeText" : newNode},
        type: "POST",
        dataType: "json",
        url: "http://localhost:8080/node",
    })
    .done(function( data, textStatus, jqXHR ) {
		console.log(newNode)
        getNodesList()
    })
    .fail(function( jqXHR, textStatus, errorThrown ) {
        console.log( "Fail " +  textStatus);
    });
}
function sendMessage(){
    newMessage = document.getElementById("messageToSend").value
    $.ajax({
        data: {"messageText" : newMessage},
        type: "POST",
        dataType: "json",
        url: "http://localhost:8080/messages",
    })
    .done(function( data, textStatus, jqXHR ) {
        getMessagesList()
    })
    .fail(function( jqXHR, textStatus, errorThrown ) {
        console.log( "Fail " +  textStatus);
    });
}
