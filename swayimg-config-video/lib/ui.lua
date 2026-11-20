-- lib/ui.lua — mensajes de estado y ayuda.

local M = {}

function M.status(msg)
  swayimg.text.status = msg
end

local HELP = [[
Galería de vídeos — videocli

  Enter   reproducir selección en cola (espacio para marcar varias)
  Shift+Enter   lanzar cada vídeo como proceso independiente
  m       reproducir solo audio (cola única)
  s       escanear carpeta (selector fuzzel)
  o / c   filtrar por categoría (reemplaza el filtro de directorio)
  d       navegar directorios (reemplaza el filtro de categoría)
  Shift+d   importar URLs (vi, una por línea; solo metadatos)
  Ctrl+d    descargar el vídeo de la entrada importada
  t       añadir/crear categoría al vídeo
  Shift+t   quitar categoría al vídeo
  r       regenerar miniatura
  i       info del vídeo
  Shift+f   gestionar categorías globalmente
  D       importar URLs (vi, una por línea; solo metadatos)
  Shift+D   descargar el vídeo de la entrada importada
  x       eliminar (fuzzel: vista / BD / local / todo)
  f       pantalla completa
  + / -   tamaño de miniaturas (64-512 px)
  Escape  salir
]]

function M.help()
  M.status(HELP)
end

return M
