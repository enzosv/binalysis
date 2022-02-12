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

async function matchCoins(binance, coingeckolist) {
    var token_ids = []
    var coins = {}
    for ([symbol, asset] of Object.entries(binance)) {
        if (asset.pairs == undefined) {
            continue
        }
        for (var i = 0; i < coingeckolist.length; i++) {
            let coin = coingeckolist[i]
            if (coin.id.includes("wormhole")) {
                // it's never this
                continue
            }
            if (coin.symbol.toLowerCase() == symbol.toLowerCase()) {
                token_ids.push(coin.id)
                coins[symbol.toLowerCase()] = { usd_market_cap: 0 }
            }
        }
    }
    const priceurl = 'https://api.coingecko.com/api/v3/simple/price?ids=' + token_ids.join(",") + '&vs_currencies=usd&include_24hr_change=true&include_market_cap=true'
    const priceRequest = await fetch(priceurl)
    console.log(priceurl)
    let priceResponse = await priceRequest.json()
    for ([id, val] of Object.entries(priceResponse)) {
        for (var i = 0; i < coingeckolist.length; i++) {
            let item = coingeckolist[i]
            if (item.id != id) {
                continue
            }
            let symbol = item.symbol.toLowerCase()
            if (coins[symbol].usd_market_cap < val.usd_market_cap) {
                val.id = item.id
                coins[symbol] = val
            }
        }
    }
    return coins
}

function usdOnly(binance, coins) {
    const usd_stablecoins = { "USDT": true, "BUSD": true, "USDC": true, "TUSD": true }
    var cleaned = {}
    for ([key, val] of Object.entries(binance)) {
        let coin = coins[key.toLowerCase()]
        if (val.pairs == undefined) {
            // console.log("skipping untraded " + key)
            cleaned[key] = val
            continue
        }
        if (coin == undefined) {
            // console.log("skipping uknown price " + key)
            continue
        }
        var merged = undefined
        for ([kk, vv] of Object.entries(val.pairs)) {
            if (!kk in usd_stablecoins) {
                continue
            }
            delete val.pairs[kk]
            if (merged == undefined) {
                merged = vv
                continue
            }
            merged.buy_qty += vv.buy_qty
            merged.cost += vv.cost
            merged.sell_qty += vv.sell_qty
            merged.revenue += vv.revenue
            if (new Date(merged.earliest_trade.Time) < new Date(vv.earliest_trade.Time)) {
                merged.earliest_trade = vv.earliest_trade
            }
            if (new Date(merged.latest_trade.Time) < new Date(vv.latest_trade.Time)) {
                merged.latest_trade = vv.latest_trade
            }
        }
        // remove non usd pairs
        val.pairs = { "USD": merged }
        val.coin = coin
        cleaned[key] = val
    }
    return cleaned
}

async function populateTable(balance, status) {
    binance = balance.binance;
    if (binance == undefined) {
        status.className = "text-danger"
        status.innerHTML = "Something went wrong. Try providing your secret key and updating."
        return
    }

    window.history.replaceState(null, null, window.origin + "?key=" + document.getElementById("key").value);
    if (Object.keys(binance).length < 1) {
        status.className = "text-warning"
        status.innerHTML = "No trades found. Try providing your secret key and updating."
        return
    }
    let coingeckoRequest = await fetch('https://api.coingecko.com/api/v3/coins/list')
    const coingecko = await coingeckoRequest.json();
    let coins = await matchCoins(balance.binance, coingecko)
    binance = usdOnly(binance, coins)

    if ($.fn.dataTable.isDataTable('#main')) {
        $('#main').DataTable().destroy()
    }

    let tbody = document.getElementById("balances")
    tbody.innerHTML = ""

    let usd_format = new Intl.NumberFormat(`en-US`, {
        currency: `USD`,
        style: 'currency',
    })
    for ([key, val] of Object.entries(binance)) {

        if (val.pairs == undefined) {
            tbody.innerHTML += `<tr>
            <td data-search="${key}">${key}</td>
            <td data-search="0"><div class='loader'></td>
            <td data-order="-1"></td>
            <td></td>
            <td></td>
            </tr>`
            continue
        }
        let coin = coins[key.toLowerCase()]
        var change = undefined
        var change_color = ""
        if (!isNaN(coin.usd_24h_change)) {
            change = coin.usd_24h_change
            change_color = (change > 0) ? "text-success" : "text-danger"
        }
        for ([kk, vv] of Object.entries(val.pairs)) {
            let buy = (vv.cost / vv.buy_qty)
            let sell = (vv.revenue / vv.sell_qty)
            var dif = undefined
            var dif_color = ""
            if (!isNaN(buy)) {
                dif = coin.usd - buy
                dif_color = (dif > 0) ? "text-success" : "text-danger"
            }
            tbody.innerHTML += `<tr>
                <td data-search="${key}"><a href="https://www.coingecko.com/en/coins/${coin.id}">${key}</a></td>
                <td data-search="${val.balance*coin.usd}">${(isNaN(buy)) ? "" : usd_format.format(buy)}</td>
                <td>${(isNaN(sell)) ? "" : usd_format.format(sell)}</td>
                <td data-order="${change ?? 0}">
                    ${(isNaN(coin.usd)) ? "" : usd_format.format(coin.usd)}
                    <small class='${change_color}'>${isNaN(change) ? "" : "(" + change.toFixed(2) + "%)"}</small>
                </td>
                <td class=${dif_color}>${(isNaN(dif)) ? "" : usd_format.format(dif)} </td>
            </tr>`
        }
    }
    
    $('#main').DataTable({
        paging: false,
        ordering: true,
        order: [[3, "desc"]]
    })
    

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
    $.fn.dataTable.ext.search.push(
        function( settings, data, dataIndex ) {                
            let min = (document.getElementById('hide-small').checked) ? 10 : -1
            console.log(min)
            return data[1] > min
        }
    )
    
    $('#search').on('keyup', function () {
        if (!$.fn.dataTable.isDataTable('#main')) {
            return
        }
        var table = $('#main').DataTable()
        table.search(this.value).draw();
    });
    $('#hide-small').on('change', function () {
        if (!$.fn.dataTable.isDataTable('#main')) {
            return
        }
        $('#main').DataTable().draw()
    });
    $('#main tbody').on('click', 'tr', function () {
        var table = $('#main').DataTable()
        var data = table.row(this).data();
        let key = data[0]["@data-search"]
        let asset = binance[key]
        let buy = asset.pairs.USD.cost / asset.pairs.USD.buy_qty
        if (isNaN(buy)) {
            return
        }
        let sell = asset.pairs.USD.revenue / asset.pairs.USD.sell_qty
        let coin = asset.coin
        let price = coin.usd
        let change = (isNaN(coin.change)) ? 0 : coin.change

        let profit = asset.pairs.USD.revenue - asset.pairs.USD.cost + asset.balance * price
        let profit_color = (profit > 0) ? "text-success" : "text-danger"
        let change_color = (change > 0) ? "text-success" : "text-danger"
        let dif = price - buy
        let dif_color = (dif > 0) ? "text-success" : "text-danger"

        let usd_format = new Intl.NumberFormat(`en-US`, {
            currency: `USD`,
            style: 'currency',
        })
        $("#exampleModal").modal("show");
        $("#modal-header").html(data[0].display)
        $("#modal-body").html(`
            <p>
            Average Buy: ${usd_format.format(buy)}<br>
            Average Sell: ${(isNaN(sell)) ? "Unsold" : usd_format.format(sell)}<br>
            Price: ${price} <label class="${change_color}">(${change.toFixed(2)})</label><br>
            Current - Buy: <label class="${dif_color}">${usd_format.format(dif)}</label><br>
            <br>
            Balance: ${asset.balance} (${usd_format.format(asset.balance * price)})<br>
            <small class="text-muted">May be inaccurate</small><br>
            Cost: ${usd_format.format(asset.pairs.USD.cost)}<br>
            Revenue: ${usd_format.format(asset.pairs.USD.revenue)}<br>
            <br>
            Profit: <label class="${profit_color}">${usd_format.format(profit)}</label><br>
            <small class="text-muted">${usd_format.format(asset.pairs.USD.revenue)}+${usd_format.format(asset.balance * price)}-${usd_format.format(asset.pairs.USD.cost)}</small><br>
            First traded: ${new Date(asset.pairs.USD.earliest_trade.Time).toLocaleDateString('en-us', { year: "numeric", month: "short", day: "numeric" })}<br>
            Last traded: ${new Date(asset.pairs.USD.latest_trade.Time).toLocaleDateString('en-us', { year: "numeric", month: "short", day: "numeric" })}
        `)
    });
});