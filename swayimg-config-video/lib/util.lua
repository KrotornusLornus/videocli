-- lib/util.lua — utilidades de shell y texto para el modo vídeos.

local M = {}

-- Directorio donde vive este fichero de configuración.
function M.here()
  local src = debug.getinfo(1, "S").source
  if src:sub(1, 1) == "@" then
    return (src:sub(2):match("^(.*/)[^/]+$") or "./") .. "/"
  end
  return "./"
end

-- Escapa una cadena para pasarla como argumento al shell.
function M.quote(s)
  return "'" .. tostring(s):gsub("'", "'\\''") .. "'"
end

-- Lee las líneas de stdout de un comando (stderr a /dev/null).
function M.lines(cmd)
  local out = {}
  local p = io.popen(cmd .. " 2>/dev/null", "r")
  if not p then return out end
  for line in p:lines() do
    table.insert(out, line)
  end
  p:close()
  return out
end

-- Ejecuta un comando sin bloquear (para acciones de larga duración).
function M.async(cmd)
  os.execute(cmd .. " >/dev/null 2>&1 &")
end

-- Expande ~ y ~/ con el HOME del usuario.
function M.expand(path)
  local home = os.getenv("HOME") or ""
  if path == "~" then return home end
  if path:sub(1, 2) == "~/" then return home .. path:sub(2) end
  return path
end

-- Selector de menú con fuzzel. Devuelve la opción elegida o nil si se cancela.
-- Con only_match=true no se acepta texto libre.
function M.fuzzel(prompt, items, only_match)
  local tmp = os.tmpname()
  local f = io.open(tmp, "w")
  if not f then return nil end
  for _, it in ipairs(items) do
    f:write(it .. "\n")
  end
  f:close()
  local flags = only_match and " --only-match" or ""
  local p = io.popen(("cat %s | fuzzel --dmenu%s --prompt=%s 2>/dev/null")
    :format(M.quote(tmp), flags, M.quote(prompt)), "r")
  local choice = p and p:read("l") or nil
  local ok = p and p:close() or false
  os.remove(tmp)
  if ok and choice and choice ~= "" then return choice end
  return nil
end

return M
