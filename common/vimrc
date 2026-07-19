set nocompatible
filetype off

set rtp+=~/.vim/bundle/Vundle.vim
call vundle#begin()
" git clone https://github.com/VundleVim/Vundle.vim.git ~/.vim/bundle/Vundle.vim

Plugin 'VundleVim/Vundle.vim'

Plugin 'preservim/nerdtree'
Plugin 'tomasiser/vim-code-dark'

call vundle#end()
filetype plugin indent on

if !empty(globpath(&rtp, 'colors/codedark.vim'))
	colorscheme codedark
endif

if has("syntax")
	syntax enable
endif

set nu
set hlsearch
set autoindent
set cindent
set ts=2
set sts=2
set shiftwidth=2
set expandtab
set laststatus=2
set smartcase
set smarttab
set smartindent
set ruler
set fileencodings=utf8,euc-kr
set wmnu

map <C-n> :NERDTreeToggle<CR>

" inoremap " ""<left>
" inoremap ' ''<left>
" inoremap ( ()<left>
" inoremap [ []<left>
" inoremap { {}<left>
" inoremap {<CR> {<CR>}<ESC>O
" inoremap {;<CR> {<CR>};<ESC>O

au BufReadPost *
\ if line("'\"") > 0 && line("'\"") <= line("$") |
\ exe "norm g`\"" |
\ endif
