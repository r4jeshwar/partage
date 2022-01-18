// Handle drag and drop into a dropzone_element div:
// send the files as a POST request to the server
"use strict";

// Only start once the DOM tree is ready
if(document.readyState === "complete") {
	setupzone();
} else {
	document.addEventListener("DOMContentLoaded", setupzone);
}

function setupzone() {
	let dropzone = document.getElementById("dropzone");
	let fileinput = document.getElementById("filebox");
	let fallbackform = document.getElementById("fallbackform");

	fallbackform.style.display = "none";

	dropzone.className = "dropzone";
	dropzone.innerHTML = "Click or drop file(s)";

	dropzone.onclick = function() {
		fileinput.click()
	return false;
	}

	dropzone.ondragover = function() {
		this.className = "dropzone dragover";
		return false;
	}

	dropzone.ondragleave = function() {
		this.className = "dropzone";
		return false;
	}

	dropzone.ondrop = function(e) {
		// Stop browser from simply opening that was just dropped
		e.preventDefault();
		// Restore original dropzone appearance
		this.className = "dropzone";
		sendfiles(e.dataTransfer.files)
	}

	fileinput.onchange = function(e) {
		sendfiles(this.files)
	}
}

function sendfiles(files) {
	let uploads = document.getElementById("uploads");
	let progressbar = document.createElement("progress");
	let uploadlist = document.createElement("ul");
	let uploadtext = document.createElement("textarea");
	let formData = new FormData(), xhr = new XMLHttpRequest();

	// used for clipboard only
	uploadtext.style.display = "none";

	uploads.appendChild(progressbar);
	uploads.appendChild(uploadlist);
	uploads.appendChild(uploadtext);

	formData.append("expiry", 10);
	for(let i=0; i < files.length; i++) {
		formData.append("file", files[i]);
	}

	// triggers periodically
	xhr.upload.onprogress = function(e) {
		// e.loaded - how many bytes downloaded
		// e.lengthComputable = true if the server sent Content-Length header
		// e.total - total number of bytes (if lengthComputable)

	}

	xhr.onreadystatechange = function() {
		if(xhr.readyState === XMLHttpRequest.DONE) {
			progressbar.remove();

			this.response.split(/\r?\n/).forEach(function(link) {
				let li = document.createElement("li");
				li.innerHTML = `<a href="${link}">${link}</a>`;
				uploadlist.appendChild(li);
			});
			let clippy = document.createElement("button");
			uploads.appendChild(clippy);
			clippy.innerText = " 📋 copy ";
			clippy.onclick = function(e) {
				uploadtext.select();
				document.execCommand("copy");
			}
		}
	}

	xhr.open('POST', window.location.href, true); // async = true
	xhr.send(formData);
}
