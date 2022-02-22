var balanceResponse = {"is_refreshing": true};
const go = new Go();
WebAssembly.instantiateStreaming(fetch("web.wasm"), go.importObject).then((result) => {
    go.run(result.instance);
    $(document).ready(function ($) {
        
        var urlParams = new URLSearchParams(window.location.search)
        if (urlParams.has('key')) {
            document.getElementById("key").value = urlParams.get('key')
            let key = urlParams.get('key')
            refresh(key)
        }
        $.fn.dataTable.ext.search.push(
            function( settings, data, dataIndex ) {                
                let min = (document.getElementById('hide-small').checked) ? 10 : 0
                return data[1] >= min
            }
        )
        
        $('#search').on('keyup', function () {
            if (!$.fn.dataTable.isDataTable('#main')) {
                return
            }
            // TODO: ignore filter
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
            presentModal(this)
        });
    });
});

async function refresh(key) {
    let btn = document.getElementById("refresh-btn")
    btn.disabled = true
    let status = document.getElementById("status")
    status.innerHTML = "Refreshing..."
    status.className = "text-light"
    try {
        let request = gorefresh(key, window.location.origin+"/latest", balanceResponse.is_refreshing)
        balanceResponse = await request
        
    } catch(err){
        balanceResponse = null
        console.error(err)
        document.getElementById("balances").innerHTML = ""
        btn.disabled = false
        status.className = "text-warning"
        status.innerHTML = "No trades found. Try providing your secret key and updating."
        generateDownloadable({})
    }
    window.history.replaceState(null, null, window.origin + "?key=" + document.getElementById("key").value);
    populateTable(balanceResponse.binance)
    generateDownloadable(balanceResponse)
    status.className = "text-light"
    status.innerHTML = "Last updated: " + new Date(balanceResponse.last_update).toLocaleDateString('en-us', { year: "numeric", month: "short", day: "numeric", hour: "numeric", minute: "numeric" })
    btn.disabled = false
    setTimeout(function(){ refresh(key); }, 120000);
}

function generateDownloadable(balance) {
    let dataStr = "data:text/json;charset=utf-8," + encodeURIComponent(JSON.stringify(balance, null, 2));
    let dlAnchorElem = document.getElementById('my-data');
    dlAnchorElem.setAttribute("href", dataStr);
    dlAnchorElem.setAttribute("download", "data.json");
    dlAnchorElem.innerHTML = "My data"
}

function populateTable(binance) {
    
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
        let change = val.coin.usd_24h_change
        let change_color = (change > 0) ? "text-success" : "text-danger"
        let dif_color = (val.dif > 0) ? "text-success" : "text-danger"
        tbody.innerHTML += `<tr>
            <td data-search="${key}"><a href="https://www.coingecko.com/en/coins/${val.coin.id}">${key}</a></td>
            <td data-search="${val.balance*val.coin.usd}">${(val.buy_qty <= 0) ? "" : usd_format.format(val.average_buy)}</td>
            <td>${(val.sell_qty <= 0) ? "" : usd_format.format(val.average_sell)}</td>
            <td data-order="${change ?? 0}">
                ${(val.coin.usd<=0) ? "" : usd_format.format(val.coin.usd)}
                <small class='${change_color}'>${(change <= 0) ? "" : "(" + change.toFixed(2) + "%)"}</small>
            </td>
            <td data-order="${val.percent_dif}" class=${dif_color}>${(val.buy_qty <= 0 || val.coin.usd <= 0) ? "" : usd_format.format(val.dif)} 
            <small>${(val.dif == 0 ) ? "" : "(" + val.percent_dif.toFixed(2) + "%)"}</small>
            </td>
        </tr>`
    }
    
    $('#main').DataTable({
        paging: false,
        ordering: true,
        order: [[3, "desc"]]
    })
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
        refresh(document.getElementById("key").value)
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
}

function presentModal(row) {
    
    var table = $('#main').DataTable()
    var data = table.row(row).data();
    let key = data[0]["@data-search"]
    let asset = balanceResponse.binance[key]
    console.log(asset)
    let profit_color = (asset.profit > 0) ? "text-success" : "text-danger"
    let change_color = (asset.coin.usd_24h_change > 0) ? "text-success" : "text-danger"
    let dif_color = (asset.dif > 0) ? "text-success" : "text-danger"

    let usd_format = new Intl.NumberFormat(`en-US`, {
        currency: `USD`,
        style: 'currency',
    })

    
    $("#exampleModal").modal("show");
    $("#modal-header").html(data[0].display)
    $("#modal-body").html(`
        <p>
        Average Buy: ${(asset.buy_qty<=0) ? "Unbought" :usd_format.format(asset.average_buy)}<br>
        Average Sell: ${(asset.sell_qty<=0) ? "Unsold" : usd_format.format(asset.average_sell)}<br>
        Price: ${asset.coin.usd} <label class="${change_color}">(${asset.coin.usd_24h_change.toFixed(2)})</label><br>
        Current - Buy: <label class="${dif_color}">${usd_format.format(asset.dif)} <small>(${asset.percent_dif.toFixed(2)}%)</label><br>
        <br>
        Balance: ${asset.balance} (${usd_format.format(asset.balance * asset.coin.usd)})<br>
        <small class="text-muted">May be inaccurate</small><br>
        Cost: ${usd_format.format(asset.cost)}<br>
        Revenue: ${usd_format.format(asset.revenue)}<br>
        <br>
        Profit: <label class="${profit_color}">${usd_format.format(asset.profit)}</label><br>
        <small class="text-muted">${usd_format.format(asset.revenue)}+${usd_format.format(asset.balance * asset.coin.usd)}-${usd_format.format(asset.cost)}</small><br>
        First trade: <br>
        &nbsp; ${asset.earliest_trade.IsBuyer ? "Bought" : "Sold"} ${asset.earliest_trade.Qty} ${key} for ${usd_format.format(asset.earliest_trade.Price*asset.earliest_trade.Qty)} on
        ${new Date(asset.earliest_trade.Time).toLocaleDateString('en-us', { year: "numeric", month: "short", day: "numeric" })}<br>
        Last trade: <br>
        &nbsp; ${asset.latest_trade.IsBuyer ? "Bought" : "Sold"} ${asset.latest_trade.Qty} ${key} for ${usd_format.format(asset.latest_trade.Price*asset.latest_trade.Qty)} on
        ${new Date(asset.latest_trade.Time).toLocaleDateString('en-us', { year: "numeric", month: "short", day: "numeric" })}<br>
    `)
}