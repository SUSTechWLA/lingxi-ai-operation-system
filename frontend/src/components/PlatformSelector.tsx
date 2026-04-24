import React from 'react'
import { useAppStore } from '../stores/appStore'
import { FaWeibo, FaBook, FaComments, FaNewspaper, FaPlay, FaPen } from 'react-icons/fa'
import { SiZhihu, SiKuaishou, SiBilibili, SiXiaohongshu, SiTiktok, SiBaidu } from 'react-icons/si'

const platformIcons: Record<string, React.ReactNode> = {
  douyin: <SiTiktok className="w-5 h-5" />,
  kuaishou: <SiKuaishou className="w-5 h-5" />,
  shipinhao: <FaPlay className="w-5 h-5" />,
  xiaohongshu: <SiXiaohongshu className="w-5 h-5" />,
  bilibili: <SiBilibili className="w-5 h-5" />,
  weibo: <FaWeibo className="w-5 h-5" />,
  toutiao: <FaNewspaper className="w-5 h-5" />,
  baijiahao: <FaPen className="w-5 h-5" />,
  zhihu: <SiZhihu className="w-5 h-5" />,
  gongzhonghao: <FaComments className="w-5 h-5" />,
}

const PlatformSelector: React.FC = () => {
  const platforms = useAppStore((state) => state.platforms)
  const togglePlatform = useAppStore((state) => state.togglePlatform)
  const toggleAllPlatforms = useAppStore((state) => state.toggleAllPlatforms)
  const selectedCount = platforms.filter((p) => p.enabled).length

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <h3 className="text-base font-semibold text-gray-800">发布平台</h3>
        <label className="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={selectedCount === platforms.length}
            onChange={(e) => toggleAllPlatforms(e.target.checked)}
            className="w-4 h-4 text-primary border-gray-300 rounded focus:ring-primary"
          />
          <span className="text-sm text-gray-500">全选</span>
        </label>
      </div>

      <div className="grid grid-cols-2 gap-3">
        {platforms.map((platform) => (
          <div
            key={platform.id}
            className={`flex items-center gap-3 p-3 rounded-xl border transition-all cursor-pointer ${
              platform.enabled
                ? 'border-primary/30 bg-primary/5'
                : 'border-gray-100 bg-white hover:border-gray-200'
            }`}
            onClick={() => togglePlatform(platform.id)}
          >
            <div
              className={`w-8 h-8 rounded-lg flex items-center justify-center flex-shrink-0 ${
                platform.enabled ? 'text-primary' : 'text-gray-400'
              }`}
            >
              {platformIcons[platform.id]}
            </div>
            <span
              className={`text-sm flex-1 ${
                platform.enabled ? 'text-gray-800' : 'text-gray-400'
              }`}
            >
              {platform.name}
            </span>
            <div
              className={`w-10 h-6 rounded-full relative transition-colors ${
                platform.enabled ? 'bg-primary' : 'bg-gray-200'
              }`}
            >
              <div
                className={`absolute top-1 w-4 h-4 bg-white rounded-full shadow transition-transform ${
                  platform.enabled ? 'left-5' : 'left-1'
                }`}
              />
            </div>
          </div>
        ))}
      </div>

      <div className="flex items-center justify-between mt-4">
        <span className="text-sm text-gray-500">
          已选择 {selectedCount}/{platforms.length} 个平台
        </span>
        <button className="text-sm text-primary hover:text-primary-dark">
          管理平台
        </button>
      </div>
    </div>
  )
}

export default PlatformSelector
