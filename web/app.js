const wasmBrowserInstantiate = async (wasmModuleUrl, importObject) => {
    let response = undefined;
    const buffer = pako.ungzip(await (await fetch(wasmModuleUrl)).arrayBuffer());
    if (WebAssembly.instantiateStreaming) {
        response = await WebAssembly.instantiateStreaming(
            pako.ungzip(await fetch(wasmModuleUrl)),
            importObject
        );
    } else {
        const fetchAndInstantiateTask = async () => {
            return WebAssembly.instantiate(buffer, importObject);
        };
        response = await fetchAndInstantiateTask();
    }

    return response;
};

// debugging slow load
const bin = fetch("./web.wasm.gz")

const go = new Go();

const runWasmAdd = async () => {
    const importObject = go.importObject;
    // never instantiatestreaming
    const fetchAndInstantiateTask = async () => {
        const buffer = pako.ungzip(await (await fetch("./web.wasm.gz")).arrayBuffer());
        return WebAssembly.instantiate(buffer, importObject);
    };
    const wasmModule = await fetchAndInstantiateTask();
    go.run(wasmModule.instance);
    setup()
};
runWasmAdd();

function setup() {

    $(document).ready(function ($) {
        document.getElementById("refresh-btn").disabled = false
        document.getElementById("update-btn").disabled = false
        document.getElementById("del-btn").disabled = false
        document.getElementById("status").innerHTML = ""
        var urlParams = new URLSearchParams(window.location.search)
        if (urlParams.has('key')) {
            document.getElementById("key").value = urlParams.get('key')
            let key = urlParams.get('key')
            refresh(key, true)
        }
        $.fn.dataTable.ext.search.push(
            function (settings, data, dataIndex) {
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
}

async function refresh(key, is_updating) {
    window.history.replaceState(null, null, window.origin + "?key=" + document.getElementById("key").value);
    let btn = document.getElementById("refresh-btn")
    btn.disabled = true
    let status = document.getElementById("status")
    status.innerHTML = "Refreshing..."
    status.className = "text-light"
    var balanceResponse
    try {
        let request = gorefresh(key, window.location.origin + "/latest", is_updating)
        balanceResponse = await request
        populateTable(balanceResponse.binance)
        generateDownloadable(balanceResponse)
        status.className = "text-light"
        status.innerHTML = "Last updated: " + new Date(balanceResponse.last_update).toLocaleDateString('en-us', { year: "numeric", month: "short", day: "numeric", hour: "numeric", minute: "numeric" })
        btn.disabled = false
        setTimeout(function () { refresh(key, balanceResponse.is_refreshing); }, 120000);
    } catch (err) {
        console.error(err)
        document.getElementById("balances").innerHTML = ""
        btn.disabled = false
        status.className = "text-warning"
        status.innerHTML = "No trades found. Try providing your secret key and updating."
        generateDownloadable({})
    }
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
        let table = $('#main').DataTable()
        table.clear();
        table.rows.add(binance);
        table.draw();
        return
    }
    const usd_format = new Intl.NumberFormat(`en-US`, {
        currency: `USD`,
        style: 'currency',
    })
    $("#main").DataTable({
        data: binance,
        fixedHeader: true,
        paging: false,
        ordering: true,
        order: [[3, "desc"]],
        columns: [
            {
                data: "symbol",
                render: function (data, type, row) {
                    return `<a href="https://www.coingecko.com/en/coins/${row.coin.id}">${data.toUpperCase()}</a>`
                }
            },
            {
                data: "average_buy",
                render: function (data, type, row) {
                    if (type === 'display') {
                        return (row.buy_qty <= 0) ? "" : usd_format.format(data)
                    }
                    return data
                },
                defaultContent: ""
            },
            {
                data: "average_sell",
                render: function (data, type, row) {
                    if (type === 'display') {
                        return (row.sell_qty <= 0) ? "" : usd_format.format(data)
                    }
                    return data
                }
            },
            {
                data: "coin.usd_24h_change",
                render: function (data, type, row) {
                    if (type === 'display') {
                        let change = data
                        let change_color = (change > 0) ? "text-success" : "text-danger"
                        return `${(row.coin.usd <= 0) ? "" : usd_format.format(row.coin.usd)}
                        <small class='${change_color}'>${(row.coin.usd <= 0) ? "" : "(" + change.toFixed(2) + "%)"}</small>`
                    }
                    return data

                },
                defaultContent: ""
            },
            {
                data: "percent_dif",
                render: function (data, type, row) {
                    if (type === 'display') {
                        let dif_color = (row.dif > 0) ? "text-success" : "text-danger"
                        return `<div class=${dif_color}>
                        ${(row.buy_qty <= 0 || row.coin.usd <= 0) ? "" : usd_format.format(row.dif)} 
                        <small>${(row.dif == 0) ? "" : "(" + data.toFixed(2) + "%)"}</small>
                        </div>`
                    }
                    return data
                }
            }
        ]
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
        refresh(document.getElementById("key").value, true)
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
    let asset = table.row(row).data();
    let profit_color = (asset.profit > 0) ? "text-success" : "text-danger"
    let change_color = (asset.coin.usd_24h_change > 0) ? "text-success" : "text-danger"
    let dif_color = (asset.dif > 0) ? "text-success" : "text-danger"

    let usd_format = new Intl.NumberFormat(`en-US`, {
        currency: `USD`,
        style: 'currency',
    })


    $("#exampleModal").modal("show");
    $("#modal-header").html(`<a href="https://www.coingecko.com/en/coins/${asset.coin.id}">${asset.symbol.toUpperCase()}</a>`)
    $("#modal-body").html(`
        <p>
        Average Buy: ${(asset.buy_qty <= 0) ? "Unbought" : usd_format.format(asset.average_buy)}<br>
        Average Sell: ${(asset.sell_qty <= 0) ? "Unsold" : usd_format.format(asset.average_sell)}<br>
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
        &nbsp; ${asset.earliest_trade.IsBuyer ? "Bought" : "Sold"} ${asset.earliest_trade.Qty} ${asset.symbol} for ${usd_format.format(asset.earliest_trade.Price * asset.earliest_trade.Qty)} 
        at ${usd_format.format(asset.earliest_trade.Price)}
        on ${new Date(asset.earliest_trade.Time).toLocaleDateString('en-us', { year: "numeric", month: "short", day: "numeric" })}<br>
        Last trade: <br>
        &nbsp; ${asset.latest_trade.IsBuyer ? "Bought" : "Sold"} ${asset.latest_trade.Qty} ${asset.symbol} for ${usd_format.format(asset.latest_trade.Price * asset.latest_trade.Qty)} 
        at ${usd_format.format(asset.latest_trade.Price)}
        on ${new Date(asset.latest_trade.Time).toLocaleDateString('en-us', { year: "numeric", month: "short", day: "numeric" })}<br>
    `)
}