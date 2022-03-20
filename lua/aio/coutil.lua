local coutil = {}

function coutil.yield(loop)
    local co = coroutine.running()
    loop:add_callback(function ()
        coroutine.resume(co)
    end)
    return coroutine.yield()
end

return coutil
