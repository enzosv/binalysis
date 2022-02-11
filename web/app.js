var binance;
async function refresh() {
    let btn = document.getElementById("refresh-btn")
    btn.disabled = true
    let status = document.getElementById("status")
    status.innerHTML = "Refreshing..."
    status.className = "text-light"
    document.getElementById("balances").innerHTML = ""
    let balanceRequest = await
        fetch('/latest', {
            method: 'GET',
            headers: {
                'X-API-Key': document.getElementById("key").value,
                'Accept': 'application/json',
                'pragma': 'no-cache',
                'cache-control': 'no-cache'
            }
        })
    if (balanceRequest.status == 404) {
        btn.disabled = false
        status.className = "text-warning"
        status.innerHTML = "No trades found. Try providing your secret key and updating."
        return
    }
    const balanceResponse = await balanceRequest.json();
    btn.disabled = false
    populateTable(balanceResponse, status)
}

async function populateTable(balance, status) {
    binance = balance.binance;
    if (binance == undefined) {
        status.className = "text-danger"
        status.innerHTML = "Something went wrong. Try providing your secret key and updating."
        return
    }
    let coingeckoRequest = await fetch('https://api.coingecko.com/api/v3/coins/list')
    const coingecko = await coingeckoRequest.json();
    var token_ids = []
    for ([key, val] of Object.entries(binance)) {
        for (var i = 0; i < coingecko.length; i++) {
            let coin = coingecko[i]
            if (coin.id.includes("wormhole")) {
                // it's never this
                continue
            }
            if (coin.symbol.toLowerCase() === key.toLowerCase()) {
                token_ids.push(coin.id)
                // break // no break to get prices for all same symbols
            }
        }
    }
    const priceurl = 'https://api.coingecko.com/api/v3/simple/price?ids=' + token_ids.join(",") + '&vs_currencies=usd&include_24hr_change=true&include_market_cap=true'
    const priceRequest = await fetch(priceurl)
    console.log(priceurl)
    let priceResponse = await priceRequest.json()
    let coins = {}
    for ([key, val] of Object.entries(priceResponse)) {
        for (var i = 0; i < coingecko.length; i++) {
            let coin = coingecko[i]
            if (coin.id === key) {
                let obj = { "usd": val.usd, "id": coin.id, "change": val.usd_24h_change, "cap": val.usd_market_cap }
                if (coins[coin.symbol.toLowerCase()] == undefined) {
                    coins[coin.symbol.toLowerCase()] = obj
                } else if (coins[coin.symbol.toLowerCase()].cap < obj.cap) {
                    // assume higher marketcap coin with same symbol is what we want
                    coins[coin.symbol.toLowerCase()] = obj
                }
            }
        }
    }
    console.log(coins)
    let tbody = document.getElementById("balances")
    tbody.innerHTML = ""
    if (Object.keys(binance).length < 1) {
        status.className = "text-warning"
        status.innerHTML = "No trades found. Try providing your secret key and updating."
        if (!$.fn.dataTable.isDataTable('#main')) {
            $('#main').DataTable({
                paging: false,
                ordering: true,
                order: [[3, "desc"]]
            })
        } else {
            var table = $('#main').DataTable()
            table.order([[3, "desc"]]).draw()
        }
        return
    }
    window.history.replaceState(null, null, window.origin + "?key=" + document.getElementById("key").value);
    let usd_format = new Intl.NumberFormat(`en-US`, {
        currency: `USD`,
        style: 'currency',
    })
    for ([key, val] of Object.entries(binance)) {
        let coin = coins[key.toLowerCase()]
        if(coin == undefined) {
            console.log("skipping " + key)
            continue
        }
        let price = coin.usd 
        let buy = (val["cost"] / val["buy_qty"])
        let sell = (val["revenue"] / val["sell_qty"])
        let remainingValue = val["balance"] * price
        // let profit = remainingValue+val["revenue"]-val["cost"]
        var row = `<tr >`
        row = `<td><a price=${price} change=${coin.change} href="https://www.coingecko.com/en/coins/${coin.id}">${key}</a></td>`
        // row += "<td><small>"+new Date(val["earliest_trade"]["Time"]).toLocaleDateString('en-us', { year:"numeric", month:"short", day:"numeric"})+ "<br>"+new Date(val["latest_trade"]["Time"]).toLocaleDateString('en-us', {  year:"numeric", month:"short", day:"numeric"}) +"</small></td>"
        if (!isNaN(buy)) {
            row += "<td>" + buy.toFixed(2) + "</td>"
        } else {
            row += "<td><center><div class='loader'></div></center></td>"
        }

        if (!isNaN(sell)) {
            row += "<td>" + usd_format.format(sell) +"</td>"
        } else {
            row += "<td></td>"
        }

        if (!isNaN(price)) {
            var color
            if (coin.change != undefined) {
                if (coin.change > 0) {
                    color = "text-success"
                } else {
                    color = "text-danger"
                }
                row += "<td data-order=" + coin.change + ">" + usd_format.format(price) + "<small class='" + color + "'> (" + coin.change.toFixed(2) + "%)</small></td>"
            } else {
                row += "<td>" + usd_format.format(price) + "</td>"
            }
            if (!isNaN(buy)) {
                let dif = price - buy
                color = "text-light"
                if (dif > 0) {
                    color = "text-success"
                } else {
                    color = "text-danger"
                }
                row += "<td class='" + color + "'>" + usd_format.format(dif) + "</td>"
            } else {
                row += "<td></td>"
            }

        } else {
            row += "<td></td>"
        }



        // row += "<td>"+profit.toFixed(2)+"</td>"
        // if((isNaN(sell) || sell>buy) && price>=buy) {
        //     row += "<td class='text-success' data-order="+1+">yes</td>"
        // } else if(sell>buy || price>=buy) {
        //     console.log(key, sell)
        //     row += "<td class='text-warning' data-order="+0+">eh</td>"
        // } else {
        //     row += "<td class='text-danger' data-order="+-1+">no</td>"
        // }
        row += "</tr>"
        tbody.innerHTML += row
    }
    if (!$.fn.dataTable.isDataTable('#main')) {
        $('#main').DataTable({
            paging: false,
            ordering: true,
            order: [[3, "desc"]]
        })
    } else {
        var table = $('#main').DataTable()
        table.order([[3, "desc"]]).draw()
    }

    // generate downloadable
    var dataStr = "data:text/json;charset=utf-8," + encodeURIComponent(JSON.stringify(balance, null, 2));
    var dlAnchorElem = document.getElementById('my-data');
    dlAnchorElem.setAttribute("href", dataStr);
    dlAnchorElem.setAttribute("download", "data.json");
    dlAnchorElem.innerHTML = "My data"

    status.className = "text-light"
    status.innerHTML = "Last updated: " + new Date(balance.last_update).toLocaleDateString('en-us', { year: "numeric", month: "short", day: "numeric", hour: "numeric", minute: "numeric" })

}

async function update() {
    let status = document.getElementById("status")
    status.className = "text-light"
    status.innerHTML = "Updating..."
    document.getElementById("update-btn").disabled = true
    let response = await fetch('/update', {
        method: 'POST',
        headers: {
            'X-API-Key': document.getElementById("key").value,
            'X-Secret-Key': document.getElementById("secret").value,
            'pragma': 'no-cache',
            'cache-control': 'no-cache'
        }
    })
    let result = await response.json()
    document.getElementById("update-btn").disabled = false
    if (result.error != undefined) {
        status.className = "text-danger"
        status.innerHTML = result.error
    } else {
        refresh()
        status.className = "text-light"
        status.innerHTML = "This will take a while. Check back later."
    }
}

async function del() {
    let status = document.getElementById("status")
    status.className = "text-light"
    status.innerHTML = "Deleting..."
    document.getElementById("del-btn").disabled = true
    let response = await fetch('/del', {
        method: 'DELETE',
        headers: {
            'X-API-Key': document.getElementById("key").value,
        }
    })
    document.getElementById("del-btn").disabled = false
    let result = await response.json()
    
    console.log(result)
    if (result.error != undefined) {
        status.className = "text-danger"
        status.innerHTML = "result.error"
        return
    }
    status.className = "text-light"
    status.innerHTML = "Deleted"
    document.getElementById("balances").innerHTML = ""

    // caches.open('v1').then(function (cache) {
    //     cache.delete('/latest').then(function (response) {
    //         window.location.reload()
    //     });
    // })
}

$(document).ready(function ($) {

    var urlParams = new URLSearchParams(window.location.search)
    if (urlParams.has('key')) {
        document.getElementById("key").value = urlParams.get('key')
        refresh()
    }
    $('#main tbody').on('click', 'tr', function () {
        var table = $('#main').DataTable()
        var data = table.row( this ).data();
        var temp = document.createElement('temp')
        temp.innerHTML = data[0]
        temp = temp.childNodes[0]
        let key = temp.innerHTML
        let asset = binance[key]
        let buy = val.cost/ val.buy_qty
        if(isNaN(buy)){
            return
        }
        let sell = val.revenue/val.sell_qty

        let price = temp.getAttribute("price")
        let change = parseFloat(temp.getAttribute("change"))
        
        let usd_format = new Intl.NumberFormat(`en-US`, {
            currency: `USD`,
            style: 'currency',
        })
        let profit = asset.revenue-asset.cost+asset.balance*price
        let profit_color = (profit > 0) ? "text-success" : "text-danger"
        let change_color = (change > 0) ? "text-success" : "text-danger"
        
        $("#exampleModal").modal("show");
        $("#modal-header").html(data[0])
        $("#modal-body").html(`
            <p>
            Average Buy: ${usd_format.format(buy)}<br>
            Average Sell: ${(isNaN(val.sell)) ? "Unsold" : usd_format.format(sell)}<br>
            Price: ${price} <label class="${change_color}">(${change.toFixed(2)})</label><br>
            Current - Buy: 
            <br>
            Balance: ${asset.balance} (${usd_format.format(asset.balance*price)})<br>
            Cost: ${usd_format.format(asset.cost)}<br>
            Revenue: ${usd_format.format(asset.revenue)}<br>
            <br>
            Profit: <label class="${profit_color}">${usd_format.format(profit)}</label><br>
            First traded: ${new Date(asset.earliest_trade.Time).toLocaleDateString('en-us', { year:"numeric", month:"short", day:"numeric"})}<br>
            Last traded: ${new Date(asset.latest_trade.Time).toLocaleDateString('en-us', { year:"numeric", month:"short", day:"numeric"})}
        `)
    } );
});