(function(old){
	$.fn.attr = function() {
		if(arguments.length === 0) {
			if(this.length === 0) {
				return null;
			}
			var obj = {};
			$.each(this[0].attributes, function() {
				if(this.specified) {
					obj[this.name] = this.value;
				}
			});
			return obj;
		}
		return old.apply(this, arguments);
	};
})($.fn.attr);

function jqspc(str){
	var spc1='\\^$*?.+()[]|{}';
	var spc2='~`@#%&=\'":;<>,/';
	var ar1=spc1.split(''),ar2=spc2.split('');
	for(var i=0;i<ar1.length;i++){
		str=str.replace(new RegExp("\\"+ ar1[i], "g"), "\\"+ar1[i]);
	}
	for(var i=0;i<ar2.length;i++){
		str=str.replace(new RegExp(ar2[i],"g"),"\\"+ar2[i]);
	}
	return str;
}
function runcb(fn,tm){
	if(typeof(fn)=='function'){
		if(!tm){tm=0;}
		setTimeout(function(){fn()},tm);
	}
}
function msgok(v,fn){layer.msg(v,{icon:1});runcb(fn,2000);}
function msgerr(v,fn){layer.msg(v,{icon:2,anim:6});runcb(fn,2000);}
function msgcry(v,fn){layer.msg(v,{icon:5,anim:6});runcb(fn,2000);}
function alertico(v,ico,fn){layer.alert(v,{icon:ico||'',anim:(ico==1)?0:6},function(idx){runcb(fn);layer.close(idx);});}
function alertok(v,fn){alertico(v,1,fn);}
function alerterr(v,fn){alertico(v,2,fn);}
function alertcry(v,fn){alertico(v,5,fn);}
function loading(){return layer.open({type:3});}
function hideload(){layer.closeAll('loading');}
function layconfirm(str,fn1,fn2){
	layer.confirm(str,{icon:3,title:'询问'},function(idx){
		layer.close(idx);
		if(typeof(fn1)=='function'){fn1();}
	},function(idx){
		layer.close(idx);
		if(typeof(fn2)=='function'){fn2();}
	});
}
function layinput(title,val,fn){
	return layer.prompt({
		'formType':0,
		value:val,
		title:title,
	},function(value,idx){
		if(typeof(fn)=='function') fn(value);
		layer.close(idx)
	});
}



function end_with(str,suf){
	return str.indexOf(suf, str.length - suf.length) !== -1;
}