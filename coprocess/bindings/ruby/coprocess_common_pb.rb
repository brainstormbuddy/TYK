require 'google/protobuf'

Google::Protobuf::DescriptorPool.generated_pool.build do
  add_message "coprocess.StringSlice" do
    repeated :items, :string, 1
  end
  add_enum "coprocess.HookType" do
    value :Unknown, 0
    value :Pre, 1
    value :Post, 2
    value :PostKeyAuth, 3
    value :CustomKeyCheck, 4
  end
end

module Coprocess
  StringSlice = Google::Protobuf::DescriptorPool.generated_pool.lookup("coprocess.StringSlice").msgclass
  HookType = Google::Protobuf::DescriptorPool.generated_pool.lookup("coprocess.HookType").enummodule
end
