async function refresh() {
    document.getElementById("refresh-btn").disabled = true
    const api_key = document.getElementById("key").value
    const [balanceResponse, coingeckoResponse] = await Promise.all([
        fetch('/latest', {
            method: 'GET',
            headers: {
                'X-API-Key': api_key,
                'Accept': 'application/json'
            }
        }),
        fetch('https://api.coingecko.com/api/v3/coins/list')
    ]);
    const balance = await balanceResponse.json();
    const binance = balance.binance;
    const coingecko = await coingeckoResponse.json();
    var token_ids = []
    for([key, val] of Object.entries(binance)) {
        for(var i=0; i<coingecko.length; i++){
            let coin = coingecko[i]
            if(coin.id.includes("wormhole")) {
                continue
            }
            if(coin.symbol.toLowerCase() === key.toLowerCase()) {
                token_ids.push(coin.id)
                // break // no break to get prices for all same symbols
            }
        }
    }
    const priceurl = 'https://api.coingecko.com/api/v3/simple/price?ids='+token_ids.join(",")+'&vs_currencies=usd&include_24hr_change=true&include_market_cap=true'
    const priceRequest = await fetch(priceurl)
    console.log(priceurl)
    let priceResponse = await priceRequest.json()
    let coins = {}
    for([key, val] of Object.entries(priceResponse)) {
        for(var i=0; i<coingecko.length; i++){
            let coin = coingecko[i]
            if(coin.id === key) {
                let obj = {"usd":val.usd, "id":coin.id, "change":val.usd_24h_change, "cap": val.usd_market_cap}
                if(coins[coin.symbol.toLowerCase()] == undefined) {
                    coins[coin.symbol.toLowerCase()] = obj
                } else if(coins[coin.symbol.toLowerCase()].cap < obj.cap) {
                    // assume higher marketcap coin with same symbol is what we want
                    coins[coin.symbol.toLowerCase()] = obj
                }
            }
        }
    }
    let tbody = document.getElementById("balances")
    tbody.innerHTML = ""
    if(Object.keys(binance).length < 1) {
        return
    }
    window.history.replaceState(null, null, window.origin+"?key="+api_key);
    for([key, val] of Object.entries(binance)) {
        let coin = coins[key.toLowerCase()]
        let price = coin.usd
        let buy = (val["cost"]/val["buy_qty"])
        let sell = (val["revenue"]/val["sell_qty"])
        let remainingValue = val["balance"]*price
        // let profit = remainingValue+val["revenue"]-val["cost"]
        var row = document.createElement("tr")
        row.innerHTML = "<td><a href=https://www.coingecko.com/en/coins/"+coin.id+">"+key+"</a></td>"
        // row.innerHTML += "<td><small>"+new Date(val["earliest_trade"]["Time"]).toLocaleDateString('en-us', { year:"numeric", month:"short", day:"numeric"})+ "<br>"+new Date(val["latest_trade"]["Time"]).toLocaleDateString('en-us', {  year:"numeric", month:"short", day:"numeric"}) +"</small></td>"
        row.innerHTML += "<td>"+buy.toFixed(2)+"</td>"
        if(!isNaN(sell)){
            row.innerHTML += "<td>"+sell.toFixed(2)+"</td>"
        } else {
            row.innerHTML += "<td></td>"
        }
        
        if(!isNaN(price)){
            var color
            if(coin.change != undefined) {
                if(coin.change > 0) {
                    color = "text-success"
                } else {
                    color = "text-danger"
                }
                row.innerHTML += "<td data-order="+coin.change+">"+price+"<small class='"+color+"'> ("+coin.change.toFixed(2)+"%)</small></td>"
            } else {
                row.innerHTML += "<td>"+price+"</td>"
            }
            let dif = price-buy
            color = "text-light"
            if(dif > 0) {
                color = "text-success"
            } else {
                color = "text-danger"
            }
            row.innerHTML += "<td class='"+color+"'>"+dif.toFixed(2)+"</td>"
        } else {
            row.innerHTML += "<td></td>"
        }


        
        // row.innerHTML += "<td>"+profit.toFixed(2)+"</td>"
        // if((isNaN(sell) || sell>buy) && price>=buy) {
        //     row.innerHTML += "<td class='text-success' data-order="+1+">yes</td>"
        // } else if(sell>buy || price>=buy) {
        //     console.log(key, sell)
        //     row.innerHTML += "<td class='text-warning' data-order="+0+">eh</td>"
        // } else {
        //     row.innerHTML += "<td class='text-danger' data-order="+-1+">no</td>"
        // }
        
        row.innerHTML += "<td data-order="+remainingValue+">"+(val["balance"]).toFixed(6)+" ($"+(remainingValue).toFixed(2)+")</td>"
        tbody.appendChild(row)
    }
    if (!$.fn.dataTable.isDataTable( '#main' ) ) {
        $('#main').DataTable({
            paging: false,
            ordering: true,
            order: [[ 3, "desc" ]]
        })
    } else {
        var table = $('#main').DataTable()
        table.order( [[ 3, "desc" ]] ).draw()
    }
    document.getElementById("refresh-btn").disabled = false
}

async function update() {
    document.getElementById("update-btn").disabled = true
    let response = await fetch('/update', {
        method: 'POST',
        headers: {
            'X-API-Key': document.getElementById("key").value,
            'X-Secret-Key': document.getElementById("secret").value
        }
    })
    let result = await response.json()
    document.getElementById("update-btn").disabled = false
    console.log(result)
}

async function del() {
    document.getElementById("del-btn").disabled = true
    let response = await fetch('/del', {
        method: 'DELETE',
        headers: {
            'X-API-Key': document.getElementById("key").value,
        }
    })
    let result = await response.json()
    document.getElementById("del-btn").disabled = true
    console.log(result)
}

$( document ).ready(function() {
    
    var urlParams = new URLSearchParams(window.location.search)
    if (urlParams.has('key')){
        document.getElementById("key").value = urlParams.get('key')
        refresh()
    }
});