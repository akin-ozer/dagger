# This file generated by `mix dagger.gen`. Please DO NOT EDIT.
defmodule Dagger.ModuleConfig do
  @moduledoc "Static configuration for a module (e.g. parsed contents of dagger.json)"
  @type t() :: %__MODULE__{
          dependencies: [Dagger.String.t()] | nil,
          exclude: [Dagger.String.t()] | nil,
          include: [Dagger.String.t()] | nil,
          name: Dagger.String.t(),
          root: Dagger.String.t() | nil,
          sdk: Dagger.String.t()
        }
  @derive Nestru.Decoder
  defstruct [:dependencies, :exclude, :include, :name, :root, :sdk]
end
